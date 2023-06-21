package llh

import (
	"time"

	"github.com/sirupsen/logrus"
)

var (
	DefaultVisibleLevels = []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
	}
	formatter = logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	}
)

type LogrusLokiHook struct {
	pusher *Pusher
}

type LogrusLokiHookConfig struct {
	Endpoint      string
	BatchMaxSize  int
	BatchMaxWait  time.Duration
	Labels        map[string]string
	Username      string
	Password      string
	VisibleLevels []logrus.Level
}

func New(c LogrusLokiHookConfig) *LogrusLokiHook {
	if len(c.VisibleLevels) == 0 {
		c.VisibleLevels = DefaultVisibleLevels
	}
	pusher := NewPusher(PusherConfig(c))
	return &LogrusLokiHook{pusher: pusher}
}

func (hook *LogrusLokiHook) Levels() []logrus.Level {
	return hook.pusher.VisibleLevels
}

func (hook *LogrusLokiHook) Fire(entry *logrus.Entry) error {
	message, err := formatter.Format(entry)
	if err != nil {
		return err
	}

	hook.pusher.Push(logEntry{
		Level:   entry.Level.String(),
		Time:    entry.Time,
		Message: string(message),
	})
	return nil
}

func (hook *LogrusLokiHook) Stop() {
	hook.pusher.Stop()
}
