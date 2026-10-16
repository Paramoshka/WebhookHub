(() => {
  if (window.WebhookHubTimezone) return;

  const storageKey = "webhookhub.timezone";
  const submissions = new WeakMap();

  function validTimezone(zone) {
    try {
      new Intl.DateTimeFormat("en-GB", { timeZone: zone });
      return true;
    } catch {
      return false;
    }
  }

  let browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  if (!browserZone || !validTimezone(browserZone)) browserZone = "UTC";
  let preference = "browser";
  try {
    const stored = localStorage.getItem(storageKey);
    if (stored === "browser" || (stored && validTimezone(stored))) preference = stored;
  } catch (error) {
    console.warn("Could not read the timezone preference; using the browser timezone.", error);
  }

  let zone;
  let formatter;

  function setTimezone() {
    zone = preference === "browser" ? browserZone : preference;
    formatter = new Intl.DateTimeFormat("en-GB", {
      timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit",
      hour: "2-digit", minute: "2-digit", second: "2-digit",
      hourCycle: "h23", timeZoneName: "longOffset",
    });
  }

  function dateParts(instant) {
    return Object.fromEntries(formatter.formatToParts(new Date(instant)).map(part => [part.type, part.value]));
  }

  function inputValue(instant) {
    if (!instant) return "";
    const p = dateParts(instant);
    return `${p.year.padStart(4, "0")}-${p.month}-${p.day}T${p.hour}:${p.minute}:${p.second}`;
  }

  function formatTime(element) {
    if (!element.dateTime || !Number.isFinite(Date.parse(element.dateTime))) return;
    const p = dateParts(element.dateTime);
    element.textContent = `${p.year.padStart(4, "0")}-${p.month}-${p.day} ${p.hour}:${p.minute}:${p.second} ${p.timeZoneName.replace("GMT", "UTC")}`;
    element.title = `${zone} · ${element.dateTime}`;
  }

  function render() {
    const select = document.getElementById("timezone-select");
    if (select) {
      if (!select.dataset.ready) {
        const zones = new Set(["UTC", browserZone]);
        if (Intl.supportedValuesOf) {
          for (const name of Intl.supportedValuesOf("timeZone")) zones.add(name);
        }
        if (preference !== "browser") zones.add(preference);
        select.replaceChildren(new Option(`Browser timezone (${browserZone})`, "browser"), new Option("UTC", "UTC"));
        zones.delete("UTC");
        for (const name of [...zones].sort()) select.add(new Option(name, name));
        select.dataset.ready = "true";
        select.disabled = false;
      }
      select.value = preference;
    }
    for (const element of document.querySelectorAll("time[data-local-time][datetime]")) formatTime(element);
    for (const label of document.querySelectorAll("[data-timezone-label]")) label.textContent = zone;
    const form = document.getElementById("filter-form");
    if (!form) return;
    form.elements.tz.value = zone;
    for (const name of ["from", "to"]) {
      const field = form.elements[name];
      // Keep unsubmitted edits as wall-clock values in the newly selected zone.
      if (field.dataset.displayValue === undefined || field.value === field.dataset.displayValue) {
        field.value = inputValue(field.dataset.instant);
        field.dataset.displayValue = field.value;
      }
    }
  }

  setTimezone();
  window.WebhookHubTimezone = { formatTime };

  document.addEventListener("change", (event) => {
    if (event.target.id !== "timezone-select") return;
    preference = event.target.value;
    try {
      localStorage.setItem(storageKey, preference);
    } catch (error) {
      console.warn("Could not save the timezone preference; it will apply to this page only.", error);
    }
    setTimezone();
    render();
  });

  document.addEventListener("htmx:configRequest", (event) => {
    const form = event.detail.elt;
    if (form.id !== "filter-form") return;
    event.detail.parameters.tz = zone;
    for (const name of ["from", "to"]) {
      const field = form.elements[name];
      // Preserve seconds, fractions, and the exact occurrence of a repeated time
      // when a user reapplies a shared URL without editing its date fields.
      if (field.value === field.dataset.displayValue) event.detail.parameters[name] = field.dataset.instant;
    }
  });

  document.addEventListener("htmx:beforeRequest", (event) => {
    const form = event.detail.elt;
    if (form.id !== "filter-form") return;
    submissions.set(event.detail.xhr, { from: form.elements.from.value, to: form.elements.to.value });
    const error = document.getElementById("filter-error");
    error.hidden = true;
    error.textContent = "";
  });

  document.addEventListener("htmx:afterRequest", (event) => {
    const { elt: form, xhr, successful } = event.detail;
    const submitted = submissions.get(xhr);
    if (!submitted || !form.isConnected) return;
    submissions.delete(xhr);
    if (xhr.status === 400) {
      const error = document.getElementById("filter-error");
      error.textContent = xhr.responseText.trim();
      error.hidden = false;
      error.focus();
      return;
    }
    if (!successful) return;
    const table = document.getElementById("webhooks-table");
    for (const name of ["from", "to"]) {
      const field = form.elements[name];
      if (field.value !== submitted[name] && field.value !== field.dataset.displayValue) continue;
      field.dataset.instant = table.dataset[name === "from" ? "filterFrom" : "filterTo"];
      field.value = inputValue(field.dataset.instant);
      field.dataset.displayValue = field.value;
    }
  });

  document.addEventListener("htmx:load", render);
  document.addEventListener("htmx:historyRestore", render);
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", render);
  else render();
})();
