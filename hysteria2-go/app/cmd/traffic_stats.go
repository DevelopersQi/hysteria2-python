package cmd

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apernet/hysteria/core/v2/client"
)

// localTrafficCounters records proxy payload bytes, excluding QUIC framing,
// encryption, retransmissions, and other transport overhead.
type localTrafficCounters struct {
	uplink   atomic.Uint64
	downlink atomic.Uint64
}

type localTrafficSnapshot struct {
	Tx        uint64 `json:"tx"`
	Rx        uint64 `json:"rx"`
	UpdatedAt int64  `json:"updatedAt"`
}

type localTrafficStatsClient struct {
	client.Client
	counters *localTrafficCounters
}

func (c *localTrafficStatsClient) TCP(addr string) (net.Conn, error) {
	conn, err := c.Client.TCP(addr)
	if err != nil {
		return nil, err
	}
	return &localTrafficStatsConn{Conn: conn, counters: c.counters}, nil
}

func (c *localTrafficStatsClient) UDP() (client.HyUDPConn, error) {
	conn, err := c.Client.UDP()
	if err != nil {
		return nil, err
	}
	return &localTrafficStatsUDP{HyUDPConn: conn, counters: c.counters}, nil
}

type localTrafficStatsConn struct {
	net.Conn
	counters *localTrafficCounters
}

func (c *localTrafficStatsConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.counters.downlink.Add(uint64(n))
	}
	return n, err
}

func (c *localTrafficStatsConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.counters.uplink.Add(uint64(n))
	}
	return n, err
}

type localTrafficStatsUDP struct {
	client.HyUDPConn
	counters *localTrafficCounters
}

func (c *localTrafficStatsUDP) Send(data []byte, addr string) error {
	err := c.HyUDPConn.Send(data, addr)
	if err == nil {
		c.counters.uplink.Add(uint64(len(data)))
	}
	return err
}

func (c *localTrafficStatsUDP) Receive() ([]byte, string, error) {
	data, addr, err := c.HyUDPConn.Receive()
	if err == nil {
		c.counters.downlink.Add(uint64(len(data)))
	}
	return data, addr, err
}

func (c *localTrafficCounters) snapshot() localTrafficSnapshot {
	return localTrafficSnapshot{
		Tx:        c.uplink.Load(),
		Rx:        c.downlink.Load(),
		UpdatedAt: time.Now().UnixMilli(),
	}
}

// startLocalTrafficStatsWriter publishes bounded JSON snapshots through a
// same-directory temporary file and rename. The returned function performs a
// final flush and waits for the writer to exit.
func startLocalTrafficStatsWriter(path string, counters *localTrafficCounters) func() {
	if path == "" || counters == nil {
		return func() {}
	}

	done := make(chan struct{})
	finished := make(chan struct{})
	var stopOnce sync.Once

	writeSnapshot := func() {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return
		}

		content, err := json.Marshal(counters.snapshot())
		if err != nil {
			return
		}

		file, err := os.CreateTemp(dir, ".furious-traffic-*.tmp")
		if err != nil {
			return
		}
		tempPath := file.Name()
		defer os.Remove(tempPath)

		if _, err = file.Write(content); err != nil {
			_ = file.Close()
			return
		}
		if err = file.Sync(); err != nil {
			_ = file.Close()
			return
		}
		if err = file.Close(); err != nil {
			return
		}

		// Windows rename does not replace an existing destination.
		_ = os.Remove(path)
		_ = os.Rename(tempPath, path)
	}

	go func() {
		defer close(finished)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		writeSnapshot()
		for {
			select {
			case <-ticker.C:
				writeSnapshot()
			case <-done:
				writeSnapshot()
				return
			}
		}
	}()

	return func() {
		stopOnce.Do(func() { close(done) })
		<-finished
	}
}
