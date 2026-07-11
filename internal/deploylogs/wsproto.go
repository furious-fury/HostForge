// Package deploylogs defines the JSON-framed WebSocket protocol for deployment live logs.
package deploylogs

const ProtocolVersion = 1

// Message type constants for JSON field "t".
const (
	TypeHello     = "hello"
	TypeChunk     = "chunk"
	TypeHeartbeat = "heartbeat"
	TypeResync    = "resync"
	TypeEnd       = "end"
	TypeError     = "error"
)
