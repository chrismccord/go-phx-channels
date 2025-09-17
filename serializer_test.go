package gophxchannels

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerializerJSONEncodeDecode(t *testing.T) {
	serializer := NewSerializer()

	msg := &Message{
		JoinRef: "1",
		Ref:     "2",
		Topic:   "room:lobby",
		Event:   "new_message",
		Payload: map[string]interface{}{
			"body": "Hello World",
			"user": "john",
		},
	}

	// Test encoding
	data, err := serializer.Encode(msg)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded []interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Len(t, decoded, 5)

	assert.Equal(t, "1", decoded[0])
	assert.Equal(t, "2", decoded[1])
	assert.Equal(t, "room:lobby", decoded[2])
	assert.Equal(t, "new_message", decoded[3])

	// Test decoding
	decodedMsg, err := serializer.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, msg.JoinRef, decodedMsg.JoinRef)
	assert.Equal(t, msg.Ref, decodedMsg.Ref)
	assert.Equal(t, msg.Topic, decodedMsg.Topic)
	assert.Equal(t, msg.Event, decodedMsg.Event)

	// Payload should be decoded as map[string]interface{}
	payloadMap, ok := decodedMsg.Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Hello World", payloadMap["body"])
	assert.Equal(t, "john", payloadMap["user"])
}

func TestSerializerBinaryEncodeDecode(t *testing.T) {
	serializer := NewSerializer()

	binaryData := []byte{0x01, 0x02, 0x03, 0x04}
	msg := &Message{
		JoinRef: "join1",
		Ref:     "ref1",
		Topic:   "room:binary",
		Event:   "binary_event",
		Payload: BinaryPayload{Data: binaryData},
	}

	// Test encoding
	data, err := serializer.Encode(msg)
	require.NoError(t, err)

	// Verify binary structure
	assert.Equal(t, KindPush, data[0]) // kind
	assert.Equal(t, byte(5), data[1])  // join_ref length
	assert.Equal(t, byte(4), data[2])  // ref length
	assert.Equal(t, byte(11), data[3]) // topic length
	assert.Equal(t, byte(12), data[4]) // event length

	// Test decoding
	decodedMsg, err := serializer.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, msg.JoinRef, decodedMsg.JoinRef)
	assert.Equal(t, "", decodedMsg.Ref) // pushes have no ref in binary protocol
	assert.Equal(t, msg.Topic, decodedMsg.Topic)
	assert.Equal(t, msg.Event, decodedMsg.Event)

	// Payload should be BinaryPayload
	binaryPayload, ok := decodedMsg.Payload.(BinaryPayload)
	require.True(t, ok)
	assert.Equal(t, binaryData, binaryPayload.Data)
}

func TestSerializerReplyDecode(t *testing.T) {
	serializer := NewSerializer()

	// Create a binary reply message
	data := []byte{
		KindReply,  // kind
		5,          // join_ref length
		4,          // ref length
		10,         // topic length
		2,          // event length (status)
	}

	// Add strings
	data = append(data, []byte("join1")...)      // join_ref
	data = append(data, []byte("ref1")...)       // ref
	data = append(data, []byte("room:test")...)  // topic
	data = append(data, []byte("ok")...)         // event (status)
	data = append(data, []byte{0x01, 0x02}...)   // binary response data

	decodedMsg, err := serializer.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, "join1", decodedMsg.JoinRef)
	assert.Equal(t, "ref1", decodedMsg.Ref)
	assert.Equal(t, "room:test", decodedMsg.Topic)
	assert.Equal(t, "phx_reply", decodedMsg.Event)

	// Check reply payload structure
	payloadMap, ok := decodedMsg.Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ok", payloadMap["status"])

	binaryResp, ok := payloadMap["response"].(BinaryPayload)
	require.True(t, ok)
	assert.Equal(t, []byte{0x01, 0x02}, binaryResp.Data)
}

func TestSerializerBroadcastDecode(t *testing.T) {
	serializer := NewSerializer()

	// Create a binary broadcast message
	data := []byte{
		KindBroadcast, // kind
		10,            // topic length
		8,             // event length
	}

	// Add strings
	data = append(data, []byte("room:test")...)      // topic
	data = append(data, []byte("new_msg")...)        // event
	data = append(data, []byte{0x01, 0x02, 0x03}...) // binary data

	decodedMsg, err := serializer.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, "", decodedMsg.JoinRef)
	assert.Equal(t, "", decodedMsg.Ref)
	assert.Equal(t, "room:test", decodedMsg.Topic)
	assert.Equal(t, "new_msg", decodedMsg.Event)

	binaryPayload, ok := decodedMsg.Payload.(BinaryPayload)
	require.True(t, ok)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, binaryPayload.Data)
}

func TestGetReplyPayload(t *testing.T) {
	// Test valid reply message
	msg := &Message{
		Event: "phx_reply",
		Payload: map[string]interface{}{
			"status":   "ok",
			"response": map[string]interface{}{"data": "test"},
		},
	}

	replyPayload, err := GetReplyPayload(msg)
	require.NoError(t, err)
	assert.Equal(t, "ok", replyPayload.Status)

	respMap, ok := replyPayload.Response.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test", respMap["data"])

	// Test non-reply message
	msg.Event = "new_message"
	_, err = GetReplyPayload(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a reply")

	// Test invalid payload format
	msg.Event = "phx_reply"
	msg.Payload = "invalid"
	_, err = GetReplyPayload(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid reply payload format")

	// Test missing status
	msg.Payload = map[string]interface{}{
		"response": "test",
	}
	_, err = GetReplyPayload(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing or invalid status")
}

func TestMessageIsBinary(t *testing.T) {
	// Test binary message
	msg := &Message{
		Payload: BinaryPayload{Data: []byte{0x01, 0x02}},
	}
	assert.True(t, msg.IsBinary())

	// Test non-binary message
	msg.Payload = map[string]interface{}{"key": "value"}
	assert.False(t, msg.IsBinary())
}

func TestSerializerErrors(t *testing.T) {
	serializer := NewSerializer()

	// Test decoding empty message
	_, err := serializer.Decode([]byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty message")

	// Test decoding invalid JSON
	_, err = serializer.Decode([]byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode JSON")

	// Test decoding invalid JSON array length
	jsonData, _ := json.Marshal([]interface{}{"one", "two", "three"})
	_, err = serializer.Decode(jsonData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid message format")

	// Test decoding truncated binary message
	_, err = serializer.Decode([]byte{KindPush})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")

	// Test unknown binary message kind
	_, err = serializer.Decode([]byte{99, 0, 0, 0, 0})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message kind")
}