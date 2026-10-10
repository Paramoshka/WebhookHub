(() => {
  const form = document.getElementById("dlq-bulk-replay");
  if (!form || form.selectionReady) return;
  form.selectionReady = true;

  const checkboxes = Array.from(document.querySelectorAll('input[name="ids"][form="dlq-bulk-replay"]'));
  const selectAll = form.querySelector("#dlq-select-all");
  const count = form.querySelector("#dlq-selection-count");
  const submit = form.querySelector('button[type="submit"]');
  let pending = false;

  function update() {
    const selected = checkboxes.filter((checkbox) => checkbox.checked).length;
    selectAll.checked = selected === checkboxes.length;
    selectAll.indeterminate = selected > 0 && selected < checkboxes.length;
    count.textContent = `${selected} webhook${selected === 1 ? "" : "s"} selected.`;
    submit.disabled = selected === 0;
  }

  selectAll.disabled = false;
  selectAll.addEventListener("change", () => {
    for (const checkbox of checkboxes) checkbox.checked = selectAll.checked;
    update();
  });
  for (const checkbox of checkboxes) checkbox.addEventListener("change", update);
  form.addEventListener("submit", (event) => {
    if (pending || !checkboxes.some((checkbox) => checkbox.checked)) {
      event.preventDefault();
      return;
    }
    pending = true;
    submit.disabled = true;
    submit.textContent = "Queueing delivery…";
  });
  window.addEventListener("pageshow", () => {
    if (!form.isConnected) return;
    pending = false;
    submit.textContent = "Requeue selected";
    update();
  });
  update();
})();
