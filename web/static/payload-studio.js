(() => {
  const page = document.getElementById("inspect-page");
  if (!page || page.payloadStudioReady) return;
  page.payloadStudioReady = true;

  const WRAP_KEY = "webhookhub:wrap-lines";
  // Re-highlighting runs on every delivery poll; keep large payloads plain
  // rather than rebuilding a huge DOM every few seconds.
  const MAX_HIGHLIGHT = 200000;

  const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g;

  // tokenizeJSON splits formatted JSON into classified tokens. Payload text is
  // untrusted: callers must insert token text via textContent only, never HTML.
  function tokenizeJSON(text) {
    const tokens = [];
    let last = 0;
    for (const match of text.matchAll(TOKEN)) {
      if (match.index > last) tokens.push({ text: text.slice(last, match.index), cls: "" });
      if (match[1] !== undefined) {
        tokens.push({ text: match[1], cls: match[2] ? "tok-key" : "tok-string" });
        if (match[2]) tokens.push({ text: match[2], cls: "" });
      } else {
        const cls = /^(?:true|false|null)$/.test(match[0]) ? "tok-literal" : "tok-number";
        tokens.push({ text: match[0], cls });
      }
      last = match.index + match[0].length;
    }
    if (last < text.length) tokens.push({ text: text.slice(last), cls: "" });
    return tokens;
  }

  function highlight(pre) {
    if (pre.dataset.highlighted === "1") return;
    const text = pre.textContent;
    pre.dataset.highlighted = "1";
    if (text.length > MAX_HIGHLIGHT) return;
    const fragment = document.createDocumentFragment();
    for (const token of tokenizeJSON(text)) {
      if (token.cls) {
        const span = document.createElement("span");
        span.className = token.cls;
        span.textContent = token.text;
        fragment.appendChild(span);
      } else {
        fragment.appendChild(document.createTextNode(token.text));
      }
    }
    pre.replaceChildren(fragment);
  }

  function wrapEnabled() {
    try {
      return localStorage.getItem(WRAP_KEY) === "1";
    } catch {
      return false;
    }
  }

  function applyWrap(scope) {
    const wrapped = wrapEnabled();
    for (const toggle of scope.querySelectorAll("[data-wrap-toggle]")) {
      toggle.setAttribute("aria-pressed", String(wrapped));
    }
    for (const pre of scope.querySelectorAll("[data-body-text]")) {
      pre.classList.toggle("is-wrapped", wrapped);
    }
  }

  function enhance(scope) {
    for (const pre of scope.querySelectorAll('pre[data-highlight="json"]')) highlight(pre);
    applyWrap(scope);
  }

  page.addEventListener("click", (event) => {
    const toggle = event.target.closest("[data-wrap-toggle]");
    if (!toggle) return;
    const body = toggle.closest("[data-body-view]");
    let wrapped;
    try {
      wrapped = localStorage.getItem(WRAP_KEY) !== "1";
      localStorage.setItem(WRAP_KEY, wrapped ? "1" : "0");
    } catch {
      wrapped = !body.querySelector("[data-body-text]").classList.contains("is-wrapped");
    }
    // The preference is global; keep every body section in sync, including
    // the payload section that polling never replaces.
    applyWrap(page);
  });

  // Delivery polling replaces the response section; re-apply to new nodes.
  page.addEventListener("htmx:afterSwap", () => enhance(page));
  enhance(page);
})();
