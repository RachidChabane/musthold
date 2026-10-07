package stubendpoint

// Wire data recorded from real servers: Ollama 0.35.1 serving tev1:0.8b, and a
// Kev server (kev.serve) serving kev-0.8b. The state is "the sky is green".

// RecordedOllamaChoiceRequest is a choice request as sent to Ollama.
const RecordedOllamaChoiceRequest = `{"model":"tev1:0.8b","state":"the sky is green","questions":{"color":{"type":"choice","instructions":"Which color does the text say the sky is?","criteria":{"blue":"blue","green":"green","grey":"grey"}}}}`

// RecordedOllamaChoiceReply is Ollama's reply to RecordedOllamaChoiceRequest.
const RecordedOllamaChoiceReply = `{"model":"tev1:0.8b","answers":{"color":{"type":"choice","choice":"green","probabilities":{"blue":0.0028416608947914386,"green":0.9948621078449252,"grey":0.0022962312602833944},"confidence":0.9674686077174837}},"usage":{"input_tokens":125,"output_tokens":1}}`

// RecordedOllamaNoulReply is Ollama's reply to a noul question, which carries no
// confidence.
const RecordedOllamaNoulReply = `{"model":"tev1:0.8b","answers":{"sky_is_green":{"type":"noul","noul":0.9861792840259738}},"usage":{"input_tokens":136,"output_tokens":1}}`

// RecordedKevChoiceReply is a Kev server's reply to the same choice question,
// sent with the model `kev-latest`; it adds `latency_ms`.
const RecordedKevChoiceReply = `{"model":"kev-latest","answers":{"color":{"type":"choice","choice":"green","confidence":0.8802,"probabilities":{"blue":0.0422,"green":0.9202,"grey":0.0377}}},"usage":{"input_tokens":32,"output_tokens":63},"latency_ms":29.9}`

// RecordedKevNoulReply is a Kev server's reply to a noul question.
const RecordedKevNoulReply = `{"model":"tev1:0.8b","answers":{"sky_is_green":{"type":"noul","noul":0.8516}},"usage":{"input_tokens":46,"output_tokens":24},"latency_ms":2103.3}`
