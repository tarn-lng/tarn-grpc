# tarn-grpc

gRPC client calls for [Tarn](https://github.com/tarn-lng/tarn) over
[tarn-http2](https://github.com/tarn-lng/tarn-http2), in pure Tarn.

**Plaintext HTTP/2 only** (grpc-go `grpc.NewServer()` without credentials):
use it on trusted networks. TLS gRPC needs ALPN, which Tarn does not expose yet.
Messages are opaque bytes; encode them with
[tarn-protobuf](https://github.com/tarn-lng/tarn-protobuf).

```tarn
import "grpc"
import "http2"

fn call(address &string, request &[]u8) Result<void, grpc.Error> {
    var client = try grpc.dial(address, &grpc.Options.new())
    metadata := [1]http2.Header{http2.Header.new(&"authorization", &"Bearer token")}
    reply := try client.unary(&"/package.Service/Method", &metadata, request)
    if !reply.status.ok() { print(reply.status.message) }
    return Ok(())
}
```

Streaming: `call(method, metadata)`, then `send(message)` any number of
times, `close_send()`, and `receive()` until `Incoming.Finished(status)`;
`Incoming.Message(bytes)` carries each response. `cancel()` resets the call.

- One call at a time per `Client`; calls can follow each other on it.
- Status comes from trailers (`grpc-status`, percent-decoded `grpc-message`,
  other trailing metadata), trailers-only responses, HTTP status mapping, or
  RST_STREAM. It is a value, never an abort.
- Compressed messages are rejected (no `grpc-accept-encoding` is offered).
  `message_limit` bounds received messages; exceeding it cancels the call.
- Blocking or nonblocking: `set_nonblocking(true)`, register `stream()` with
  `net.Poll`, and drive `send`/`receive`/`flush`. `pending()` reports queued
  bytes for backpressure.

## Tests

- `tarn test`: status/trailer parsing, percent decoding, metadata validation.
- `tests/run.sh path/to/tarn`: integration against real grpc-go — unary with
  metadata and protobuf payloads decoded by Go's protowire, error statuses,
  UNIMPLEMENTED, a nonblocking bidirectional stream of 200 messages larger
  than an HTTP/2 frame, sequential calls on one client, and the receive limit.
