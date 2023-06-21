package llh

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBatchMaxSize = 1000
	defaultBatchMaxWait = time.Second
	defaultEndpoint     = "http://localhost:3100"
	defaultHttpTimeout  = 10 * time.Second
	minGzipSize         = 3 * 1024
)

type PusherConfig LogrusLokiHookConfig

type Pusher struct {
	PusherConfig
	client    *http.Client
	quit      chan struct{}
	entries   chan logEntry
	waitGroup sync.WaitGroup
}

type logEntry struct {
	Time    time.Time
	Level   string
	Message string
}

type logBatch []logEntry

func (batch logBatch) Encode(labels map[string]string) stream {
	values := make([][2]string, len(batch))
	for i, e := range batch {
		values[i] = [2]string{
			strconv.FormatInt(e.Time.UnixNano(), 10),
			e.Message,
		}
	}
	return stream{
		Stream: labels,
		Values: values,
	}
}

func NewPusher(c PusherConfig) (pusher *Pusher) {
	if c.BatchMaxSize == 0 {
		c.BatchMaxSize = defaultBatchMaxSize
	}
	if c.BatchMaxWait == 0 {
		c.BatchMaxWait = defaultBatchMaxWait
	}
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	c.Endpoint = strings.TrimSuffix(c.Endpoint, "/") + "/loki/api/v1/push"

	pusher = &Pusher{
		PusherConfig: c,
		client:       &http.Client{Timeout: defaultHttpTimeout},
		quit:         make(chan struct{}),
		entries:      make(chan logEntry),
		waitGroup:    sync.WaitGroup{},
	}

	pusher.waitGroup.Add(1)
	go pusher.run()
	return
}

func (p *Pusher) Push(e logEntry) {
	p.entries <- e
}

func (p *Pusher) Stop() {
	close(p.quit)
	p.waitGroup.Wait()
}

func (p *Pusher) run() {
	batch := make([]logEntry, 0, p.BatchMaxSize)
	ticker := time.NewTimer(p.BatchMaxWait)

	defer func() {
		if len(batch) > 0 {
			p.sendQuiet(batch)
		}
		p.waitGroup.Done()
	}()

	for {
		select {
		case <-p.quit:
			return
		case entry := <-p.entries:
			batch = append(batch, entry)
			if len(batch) >= p.BatchMaxSize {
				p.sendQuiet(batch)
				batch = batch[:0]
				ticker.Reset(p.BatchMaxWait)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				p.sendQuiet(batch)
				batch = batch[:0]
			}
			ticker.Reset(p.BatchMaxWait)
		}
	}
}

type lokiPushData struct {
	Streams []stream `json:"streams"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (p *Pusher) sendQuiet(batch logBatch) {
	if err := p.send(batch); err != nil {
		log.Printf("[system] Drop log: %s", err)
	}
}

func (p *Pusher) send(batch logBatch) error {
	data := lokiPushData{
		Streams: []stream{batch.Encode(p.Labels)},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	var (
		buf      bytes.Buffer
		withGzip bool
	)
	if withGzip = len(jsonData) >= minGzipSize; withGzip {
		g := gzip.NewWriter(&buf)
		if _, err := g.Write(jsonData); err != nil {
			return fmt.Errorf("failed to gzip json: %w", err)
		}
		if err := g.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
	} else {
		buf.Write(jsonData)
	}

	req, err := http.NewRequest("POST", p.Endpoint, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if withGzip {
		req.Header.Set("Content-Encoding", "gzip")
	}

	if p.Username != "" && p.Password != "" {
		req.SetBasicAuth(p.Username, p.Password)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("recieved unexpected response code from Loki: %s", resp.Status)
	}

	return nil
}
