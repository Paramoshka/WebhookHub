(() => {
  const page = document.getElementById("dashboard-page");
  if (!page || page.dashboardControlsReady) return;
  page.dashboardControlsReady = true;

  let paused = false;
  let actionPending = false;
  const requests = new Map();
  const cancelled = new WeakSet();
  const actionMessage = page.querySelector("#dashboard-action-message");

  function refresh() {
    if (!page.isConnected || actionPending) return;
    htmx.trigger(page.querySelector("#webhooks-table"), "refresh");
    htmx.trigger(page.querySelector("#delivery-metrics"), "refresh");
  }

  function cancelReads(backgroundOnly) {
    for (const [xhr, request] of requests) {
      if (request.kind !== "action" && (!backgroundOnly || request.background)) {
        cancelled.add(xhr);
        xhr.abort();
      }
    }
  }

  function errorMessage(status) {
    if (status === 403) return "Session verification failed. Reload the page and try again.";
    if (status === 404) return "This webhook no longer exists.";
    if (status === 409) return "Delivery is already in progress. Please wait.";
    if (status === 0) return "Connection lost or request timed out. Reload to check the result before retrying.";
    return "The server could not complete the request. Reload to check the result before retrying.";
  }

  page.querySelector("#refresh-toggle").addEventListener("click", (event) => {
    paused = !paused;
    event.currentTarget.textContent = paused ? "Resume updates" : "Pause updates";
    event.currentTarget.setAttribute("aria-pressed", String(paused));
    page.querySelector("#refresh-state").textContent = paused ? "Auto-refresh paused" : "Auto-refresh on · every 5 seconds";
    if (paused) cancelReads(true);
    else refresh();
  });

  page.addEventListener("htmx:beforeRequest", (event) => {
    const { elt, xhr, target, requestConfig } = event.detail;
    const action = elt.dataset.webhookAction;
    let kind = "";
    if (action) kind = "action";
    else if (target.id === "webhooks-table") kind = "logs";
    else if (target.id === "delivery-metrics") kind = "metrics";
    if (!kind) return;
    const background = elt.id === "webhooks-table" || elt.id === "delivery-metrics";
    if (actionPending || (paused && background && requestConfig.triggeringEvent?.type !== "refresh")) {
      event.preventDefault();
      return;
    }
    if (action) {
      actionPending = true;
      cancelReads(false);
      for (const button of page.querySelectorAll("[data-webhook-action]")) button.disabled = true;
      actionMessage.className = "";
      actionMessage.textContent = action === "delete" ? "Deleting webhook…" : "Queueing delivery…";
    }
    requests.set(xhr, { kind, background, action, id: elt.dataset.webhookId });
  });

  page.addEventListener("htmx:beforeSwap", (event) => {
    if (cancelled.has(event.detail.xhr)) event.preventDefault();
  });

  page.addEventListener("htmx:afterRequest", (event) => {
    const { xhr, successful } = event.detail;
    const request = requests.get(xhr);
    if (!request) return;
    requests.delete(xhr);
    if (cancelled.has(xhr)) return;

    if (request.kind === "action") {
      actionPending = false;
      for (const button of page.querySelectorAll("[data-webhook-action]")) button.disabled = false;
      actionMessage.className = successful ? "" : "inspect-error";
      if (successful) {
        actionMessage.textContent = request.action === "delete" ? "Webhook deleted." : "Queued for delivery.";
      } else {
        actionMessage.textContent = errorMessage(xhr.status);
      }
      if (successful && request.action === "delete") page.querySelector(`#webhook-${request.id}`)?.remove();
      // Release HTMX's request lock before asking for fresh data, even while paused.
      if (successful || xhr.status === 404 || xhr.status === 409) setTimeout(refresh, 0);
      return;
    }

    const message = page.querySelector(`#${request.kind}-refresh-message`);
    message.className = successful ? "" : "inspect-error";
    message.textContent = successful ? "" : "Could not refresh. Displayed data may be out of date. Check the connection and try again.";
    if (successful) {
      const time = page.querySelector(`#${request.kind}-updated`);
      const now = new Date();
      time.dateTime = now.toISOString();
      time.textContent = now.toISOString().replace("T", " ").replace(/\.\d{3}Z$/, " UTC");
    }
  });
})();
