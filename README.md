# [Loki](https://grafana.com/oss/loki/) Log Hook for [Logrus](https://github.com/sirupsen/logrus)

## Install

`go get -u github.com/gota33/logrus-loki-hook`

## How to use

```go
package main

import (
	"github.com/sirupsen/logrus"
	llh "github.com/gota33/logrus-loki-hook"
)

func main() {
	hook := llh.New(llh.LogrusLokiHookConfig{
		Endpoint: "http://localhost:3100",
		Labels:   map[string]string{"app": "demo"},
	})
	defer hook.Stop()
	
	logrus.AddHook(hook)
	logrus.Info("Hi")
}
```