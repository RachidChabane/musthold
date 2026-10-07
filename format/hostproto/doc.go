// Package hostproto is the host protocol shared by every proof module: the
// request a module emits when a proof needs model work, the ticket and response
// that carry the answer back, the lifecycle state machine that decides what a
// module may consume, the binding and capability checks that tie a response to
// the executor that was started for it, the journal chain, the Store port, the
// decoders of the documented hook payloads of the supported harnesses, and the
// pure choice of the channel a response travels through.
//
// The package is pure: it reads no file, runs no process, opens no network
// connection and draws no randomness of its own. Entropy, the clock and the
// secret are passed in by the adapters of each module.
package hostproto
