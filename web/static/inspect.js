(() => {
  const page = document.getElementById("inspect-page");
  if (!page || page.inspectControlsReady) return;
  page.inspectControlsReady = true;

  page.addEventListener("click", async (event) => {
    const button = event.target.closest("button");
    if (!button) return;
    const body = button.closest("[data-body-view]");
    if (!body) return;

    if (button.dataset.bodyMode) {
      for (const text of body.querySelectorAll("[data-body-text]")) {
        text.hidden = text.dataset.bodyText !== button.dataset.bodyMode;
      }
      for (const toggle of body.querySelectorAll("[data-body-mode]")) {
        toggle.setAttribute("aria-pressed", String(toggle === button));
      }
      body.querySelector("[data-copy-message]").textContent = "";
    }

    if (button.hasAttribute("data-copy-body")) {
      const text = body.querySelector("[data-body-text]:not([hidden])");
      const message = body.querySelector("[data-copy-message]");
      try {
        if (!navigator.clipboard?.writeText) throw new Error("Clipboard unavailable");
        await navigator.clipboard.writeText(text.textContent);
        message.textContent = "Copied.";
        message.className = "";
      } catch (error) {
        message.textContent = "Copy unavailable. Select the text and copy it manually.";
        message.className = "inspect-error";
      }
    }
  });

  page.addEventListener("htmx:beforeRequest", (event) => {
    if (event.detail.elt.id === "inspect-replay") {
      event.detail.elt.querySelector("button").disabled = true;
      page.querySelector("#inspect-action-message").textContent = "Queueing delivery…";
    }
  });

  page.addEventListener("htmx:afterRequest", (event) => {
    const { xhr, successful, requestConfig } = event.detail;
    // After an outerHTML swap, HTMX emits afterRequest on the surviving parent.
    const elt = requestConfig.elt;
    if (elt.id === "inspect-replay") {
      const message = page.querySelector("#inspect-action-message");
      elt.querySelector("button").disabled = false;
      message.className = successful ? "" : "inspect-error";
      if (successful) {
        message.textContent = "Queued for delivery.";
      } else if (xhr.status === 409) {
        message.textContent = "Delivery is already in progress. Please wait.";
      } else if (xhr.status === 404) {
        message.textContent = "This webhook no longer exists.";
      } else if (xhr.status === 403) {
        message.textContent = "Session verification failed. Reload the page and try again.";
      } else {
        message.textContent = "Could not queue delivery. Check the connection and try again.";
      }
      if (successful || xhr.status === 409) {
        // Wait until HTMX releases the request lock shared with polling.
        setTimeout(() => {
          if (page.isConnected) htmx.trigger(page.querySelector("#inspect-delivery"), "refresh");
        }, 0);
      }
    }
    if (elt.id === "inspect-delivery") {
      const message = page.querySelector("#inspect-refresh-message");
      message.className = successful ? "" : "inspect-error";
      message.textContent = successful ? "" : "Delivery details could not be refreshed. Showing the last loaded data.";
    }
  });
})();
