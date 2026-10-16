(() => {
  if (window.WebhookHubTimezonePicker || !HTMLElement.prototype.showPopover) return;

  let select;
  let trigger;
  let popup;
  let search;
  let list;
  let entries = [];
  let matches = [];
  let active = -1;

  function normalize(value) {
    return value.toLowerCase().replaceAll("_", " ");
  }

  function position() {
    if (!popup?.matches(":popover-open") || !trigger?.isConnected) return;
    const rect = trigger.getBoundingClientRect();
    const left = Math.max(12, Math.min(rect.left, innerWidth - popup.offsetWidth - 12));
    let top = rect.bottom + 8;
    if (top + popup.offsetHeight > innerHeight - 12) top = rect.top - popup.offsetHeight - 8;
    top = Math.max(12, Math.min(top, innerHeight - popup.offsetHeight - 12));
    popup.style.left = `${left}px`;
    popup.style.top = `${top}px`;
  }

  function highlight(index) {
    list.querySelector('[aria-selected="true"]')?.setAttribute("aria-selected", "false");
    active = matches.length ? Math.max(0, Math.min(index, matches.length - 1)) : -1;
    const option = list.children[active];
    if (!option) {
      search.removeAttribute("aria-activedescendant");
      return;
    }
    option.setAttribute("aria-selected", "true");
    search.setAttribute("aria-activedescendant", option.id);
    if (popup.matches(":popover-open")) option.scrollIntoView({ block: "nearest" });
  }

  function filter() {
    const words = normalize(search.value).trim().split(/\s+/).filter(Boolean);
    matches = entries.filter(entry => words.every(word => entry.search.includes(word)));
    const options = document.createDocumentFragment();
    for (const [index, entry] of matches.entries()) {
      const option = document.createElement("li");
      option.id = `timezone-option-${index}`;
      option.textContent = entry.label;
      option.dataset.value = entry.value;
      option.dataset.current = String(entry.value === select.value);
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", "false");
      options.append(option);
    }
    list.replaceChildren(options);
    document.getElementById("timezone-empty").hidden = matches.length > 0;
    const current = words.length ? 0 : matches.findIndex(entry => entry.value === select.value);
    highlight(Math.max(0, current));
    position();
  }

  function close(restoreFocus = true) {
    if (!popup?.matches(":popover-open")) return;
    popup.hidePopover();
    trigger.setAttribute("aria-expanded", "false");
    search.setAttribute("aria-expanded", "false");
    if (restoreFocus && trigger.isConnected) trigger.focus({ preventScroll: true });
  }

  function open(query = "") {
    search.value = query;
    filter();
    popup.showPopover();
    trigger.setAttribute("aria-expanded", "true");
    search.setAttribute("aria-expanded", "true");
    position();
    search.focus({ preventScroll: true });
    highlight(active);
  }

  function choose(value) {
    select.value = value;
    close();
    select.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function handleSearchKey(event) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      highlight(active + (event.key === "ArrowDown" ? 1 : -1));
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (matches[active]) choose(matches[active].value);
    } else if (event.key === "Escape") {
      event.preventDefault();
      close();
    } else if (event.key === "Tab") {
      close(false);
    }
  }

  function refresh() {
    select = document.getElementById("timezone-select");
    if (!select || select.disabled) return;
    trigger = document.getElementById("timezone-picker");
    popup = document.getElementById("timezone-popup");
    search = document.getElementById("timezone-search");
    list = document.getElementById("timezone-options");
    const text = select.selectedOptions[0].textContent;
    const choice = document.getElementById("timezone-choice");
    if (choice.textContent !== text) choice.textContent = text;
    select.hidden = true;
    trigger.hidden = false;
    document.getElementById("timezone-label").htmlFor = trigger.id;
    if (trigger.timezonePickerReady) return;
    trigger.timezonePickerReady = true;
    const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    entries = Array.from(select.options, option => {
      const label = option.value === "browser" ? `${option.textContent} (${browserZone})` : option.textContent;
      return { value: option.value, label, search: normalize(label + " " + option.value) };
    });
    trigger.addEventListener("click", () => {
      if (popup.matches(":popover-open")) close();
      else open();
    });
    trigger.addEventListener("keydown", (event) => {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        open();
      } else if (event.key.length === 1 && event.key !== " " && !event.ctrlKey && !event.metaKey && !event.altKey) {
        event.preventDefault();
        open(event.key);
      }
    });
    search.addEventListener("input", filter);
    search.addEventListener("keydown", handleSearchKey);
    list.addEventListener("click", (event) => {
      const option = event.target.closest('[role="option"]');
      if (option) choose(option.dataset.value);
    });
    popup.addEventListener("toggle", (event) => {
      if (event.target === popup && event.newState === "closed") {
        trigger.setAttribute("aria-expanded", "false");
        search.setAttribute("aria-expanded", "false");
      }
    });
  }

  window.WebhookHubTimezonePicker = { refresh };
  document.addEventListener("htmx:beforeSwap", (event) => {
    if (event.detail.shouldSwap && trigger && event.detail.target.contains(trigger)) close(false);
  });
  window.addEventListener("resize", position);
  document.addEventListener("scroll", position, true);
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", refresh);
  else refresh();
})();
