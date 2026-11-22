// The playground's interpreter: Evie compiled to WebAssembly, running in this
// worker so a busy program never freezes the page. The page posts
// {type: "run", code, stdin}; Evie posts back "ready", "out" chunks and a
// "done" message. To stop a program the page terminates the worker.
importScripts("wasm_exec.js");

const go = new Go();

async function load() {
  try {
    const result = await WebAssembly.instantiateStreaming(fetch("evie.wasm"), go.importObject);
    return result.instance;
  } catch {
    // a server that sends the wrong content type defeats streaming
    const bytes = await (await fetch("evie.wasm")).arrayBuffer();
    return (await WebAssembly.instantiate(bytes, go.importObject)).instance;
  }
}

load()
  .then((instance) => go.run(instance)) // Evie posts "ready" itself
  .catch((err) => postMessage({ type: "failed", error: String(err) }));

onmessage = (event) => {
  const msg = event.data;
  if (msg.type === "run") self.evieRun(msg.code, msg.stdin ?? "");
};
