package cmd

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type trafficTestConn struct {
	reader *bytes.Reader
	writer bytes.Buffer
}

func (c *trafficTestConn) Read(p []byte) (int, error)         { return c.reader.Read(p) }
func (c *trafficTestConn) Write(p []byte) (int, error)        { return c.writer.Write(p) }
func (c *trafficTestConn) Close() error                       { return nil }
func (c *trafficTestConn) LocalAddr() net.Addr                { return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (c *trafficTestConn) RemoteAddr() net.Addr               { return &net.IPAddr{IP: net.IPv4(127, 0, 0, 2)} }
func (c *trafficTestConn) SetDeadline(_ time.Time) error      { return nil }
func (c *trafficTestConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *trafficTestConn) SetWriteDeadline(_ time.Time) error { return nil }

type trafficTestUDP struct{}

func (*trafficTestUDP) Send([]byte, string) error { return nil }
func (*trafficTestUDP) Receive() ([]byte, string, error) {
	return []byte("udp-down"), "example.com:53", nil
}
func (*trafficTestUDP) Close() error { return nil }

func TestLocalTrafficWrappersCountProxyPayload(t *testing.T) {
	counters := &localTrafficCounters{}
	conn := &localTrafficStatsConn{
		Conn:     &trafficTestConn{reader: bytes.NewReader([]byte("tcp-down"))},
		counters: counters,
	}

	if _, err := conn.Write([]byte("tcp-up")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 32)
	if _, err := conn.Read(buffer); err != nil {
		t.Fatal(err)
	}

	udp := &localTrafficStatsUDP{
		HyUDPConn: &trafficTestUDP{},
		counters:  counters,
	}
	if err := udp.Send([]byte("udp-up"), "example.com:53"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := udp.Receive(); err != nil {
		t.Fatal(err)
	}

	snapshot := counters.snapshot()
	if snapshot.Tx != uint64(len("tcp-up")+len("udp-up")) {
		t.Fatalf("unexpected uplink counter: %d", snapshot.Tx)
	}
	if snapshot.Rx != uint64(len("tcp-down")+len("udp-down")) {
		t.Fatalf("unexpected downlink counter: %d", snapshot.Rx)
	}
}

func TestLocalTrafficWriterFlushesFinalSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.json")
	counters := &localTrafficCounters{}
	counters.uplink.Store(123)
	counters.downlink.Store(456)

	stop := startLocalTrafficStatsWriter(path, counters)
	stop()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot localTrafficSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Tx != 123 || snapshot.Rx != 456 || snapshot.UpdatedAt <= 0 {
		t.Fatalf("unexpected traffic snapshot: %+v", snapshot)
	}
}
