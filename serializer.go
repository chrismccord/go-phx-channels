package gophxchannels

import (
	"encoding/json"
	"fmt"
)

const (
	HeaderLength = 1
	MetaLength   = 4
)

// Message kinds for binary protocol
const (
	KindPush      byte = 0
	KindReply     byte = 1
	KindBroadcast byte = 2
)

// Message represents a Phoenix channel message
type Message struct {
	JoinRef string      `json:"join_ref"`
	Ref     string      `json:"ref"`
	Topic   string      `json:"topic"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

// BinaryPayload represents a binary payload
type BinaryPayload struct {
	Data []byte
}

// IsBinary checks if the message payload is binary
func (m *Message) IsBinary() bool {
	_, ok := m.Payload.(BinaryPayload)
	return ok
}

// Serializer handles encoding/decoding of Phoenix messages
type Serializer struct{}

// NewSerializer creates a new serializer instance
func NewSerializer() *Serializer {
	return &Serializer{}
}

// Encode encodes a message for transmission
func (s *Serializer) Encode(msg *Message) ([]byte, error) {
	if msg.IsBinary() {
		return s.binaryEncode(msg)
	}
	return s.jsonEncode(msg)
}

// Decode decodes a received message
func (s *Serializer) Decode(data []byte) (*Message, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message")
	}

	// Check if it's a binary message by looking at the first byte
	if data[0] == KindPush || data[0] == KindReply || data[0] == KindBroadcast {
		return s.binaryDecode(data)
	}
	return s.jsonDecode(data)
}

// jsonEncode encodes a message as JSON
func (s *Serializer) jsonEncode(msg *Message) ([]byte, error) {
	payload := []interface{}{msg.JoinRef, msg.Ref, msg.Topic, msg.Event, msg.Payload}
	return json.Marshal(payload)
}

// jsonDecode decodes a JSON message
func (s *Serializer) jsonDecode(data []byte) (*Message, error) {
	var payload []interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	if len(payload) != 5 {
		return nil, fmt.Errorf("invalid message format: expected 5 elements, got %d", len(payload))
	}

	msg := &Message{}

	if payload[0] != nil {
		if str, ok := payload[0].(string); ok {
			msg.JoinRef = str
		}
	}

	if payload[1] != nil {
		if str, ok := payload[1].(string); ok {
			msg.Ref = str
		}
	}

	if str, ok := payload[2].(string); ok {
		msg.Topic = str
	} else {
		return nil, fmt.Errorf("invalid topic type")
	}

	if str, ok := payload[3].(string); ok {
		msg.Event = str
	} else {
		return nil, fmt.Errorf("invalid event type")
	}

	msg.Payload = payload[4]

	return msg, nil
}

// binaryEncode encodes a message with binary payload
func (s *Serializer) binaryEncode(msg *Message) ([]byte, error) {
	binaryPayload, ok := msg.Payload.(BinaryPayload)
	if !ok {
		return nil, fmt.Errorf("payload is not binary")
	}

	joinRef := msg.JoinRef
	ref := msg.Ref
	topic := msg.Topic
	event := msg.Event

	// Calculate metadata length
	metaLength := MetaLength + len(joinRef) + len(ref) + len(topic) + len(event)

	// Create header buffer
	header := make([]byte, HeaderLength+metaLength)
	offset := 0

	// Write kind
	header[offset] = KindPush
	offset++

	// Write lengths
	header[offset] = byte(len(joinRef))
	offset++
	header[offset] = byte(len(ref))
	offset++
	header[offset] = byte(len(topic))
	offset++
	header[offset] = byte(len(event))
	offset++

	// Write strings
	copy(header[offset:], joinRef)
	offset += len(joinRef)
	copy(header[offset:], ref)
	offset += len(ref)
	copy(header[offset:], topic)
	offset += len(topic)
	copy(header[offset:], event)
	offset += len(event)

	// Combine header and payload
	result := make([]byte, len(header)+len(binaryPayload.Data))
	copy(result, header)
	copy(result[len(header):], binaryPayload.Data)

	return result, nil
}

// binaryDecode decodes a binary message
func (s *Serializer) binaryDecode(data []byte) (*Message, error) {
	if len(data) < HeaderLength {
		return nil, fmt.Errorf("message too short")
	}

	kind := data[0]
	switch kind {
	case KindPush:
		return s.decodePush(data)
	case KindReply:
		return s.decodeReply(data)
	case KindBroadcast:
		return s.decodeBroadcast(data)
	default:
		return nil, fmt.Errorf("unknown message kind: %d", kind)
	}
}

// decodePush decodes a push message
func (s *Serializer) decodePush(data []byte) (*Message, error) {
	if len(data) < HeaderLength+MetaLength-1 {
		return nil, fmt.Errorf("push message too short")
	}

	joinRefSize := int(data[1])
	topicSize := int(data[2])
	eventSize := int(data[3])

	offset := HeaderLength + MetaLength - 1 // pushes have no ref

	if len(data) < offset+joinRefSize+topicSize+eventSize {
		return nil, fmt.Errorf("push message truncated")
	}

	joinRef := string(data[offset : offset+joinRefSize])
	offset += joinRefSize

	topic := string(data[offset : offset+topicSize])
	offset += topicSize

	event := string(data[offset : offset+eventSize])
	offset += eventSize

	payload := BinaryPayload{Data: data[offset:]}

	return &Message{
		JoinRef: joinRef,
		Ref:     "", // pushes have no ref
		Topic:   topic,
		Event:   event,
		Payload: payload,
	}, nil
}

// decodeReply decodes a reply message
func (s *Serializer) decodeReply(data []byte) (*Message, error) {
	if len(data) < HeaderLength+MetaLength {
		return nil, fmt.Errorf("reply message too short")
	}

	joinRefSize := int(data[1])
	refSize := int(data[2])
	topicSize := int(data[3])
	eventSize := int(data[4])

	offset := HeaderLength + MetaLength

	if len(data) < offset+joinRefSize+refSize+topicSize+eventSize {
		return nil, fmt.Errorf("reply message truncated")
	}

	joinRef := string(data[offset : offset+joinRefSize])
	offset += joinRefSize

	ref := string(data[offset : offset+refSize])
	offset += refSize

	topic := string(data[offset : offset+topicSize])
	offset += topicSize

	event := string(data[offset : offset+eventSize])
	offset += eventSize

	binaryData := data[offset:]

	// For replies, the payload is wrapped with status and response
	payload := map[string]interface{}{
		"status":   event,
		"response": BinaryPayload{Data: binaryData},
	}

	return &Message{
		JoinRef: joinRef,
		Ref:     ref,
		Topic:   topic,
		Event:   "phx_reply",
		Payload: payload,
	}, nil
}

// decodeBroadcast decodes a broadcast message
func (s *Serializer) decodeBroadcast(data []byte) (*Message, error) {
	if len(data) < HeaderLength+2 {
		return nil, fmt.Errorf("broadcast message too short")
	}

	topicSize := int(data[1])
	eventSize := int(data[2])
	offset := HeaderLength + 2

	if len(data) < offset+topicSize+eventSize {
		return nil, fmt.Errorf("broadcast message truncated")
	}

	topic := string(data[offset : offset+topicSize])
	offset += topicSize

	event := string(data[offset : offset+eventSize])
	offset += eventSize

	payload := BinaryPayload{Data: data[offset:]}

	return &Message{
		JoinRef: "",
		Ref:     "",
		Topic:   topic,
		Event:   event,
		Payload: payload,
	}, nil
}

// ReplyPayload represents the structure of a reply payload
type ReplyPayload struct {
	Status   string      `json:"status"`
	Response interface{} `json:"response"`
}

// GetReplyPayload extracts a reply payload from a message
func GetReplyPayload(msg *Message) (*ReplyPayload, error) {
	if msg.Event != "phx_reply" {
		return nil, fmt.Errorf("message is not a reply")
	}

	payloadMap, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid reply payload format")
	}

	status, ok := payloadMap["status"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid status in reply")
	}

	return &ReplyPayload{
		Status:   status,
		Response: payloadMap["response"],
	}, nil
}