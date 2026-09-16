// Stepping is intentionally narrower than the builder's quantity grammar.
// Unsupported quantities and templates remain editable as text, never coerced.
const resourceStepper = (() => {
  function parse(kind, value) {
    const text = String(value ?? '').trim();
    if (!text) return 0;
    const match = kind === 'cpu'
      ? text.match(/^(\d+(?:\.\d+)?)(m)?$/)
      : text.match(/^(\d+(?:\.\d+)?)(Ki|Mi|Gi|Ti)?$/);
    if (!match) return null;
    const factor = kind === 'cpu' ? (match[2] ? 1 : 1000)
      : ({Ki: 1024, Mi: 1048576, Gi: 1073741824, Ti: 1099511627776}[match[2]] || 1);
    const amount = Number(match[1]) * factor;
    return Number.isSafeInteger(amount) && amount >= 0 ? amount : null;
  }
  function next(kind, value, step, direction) {
    const amount = parse(kind, value);
    if (amount === null || ![1, -1].includes(direction) || !Number.isSafeInteger(step) || step <= 0) return null;
    const delta = step * (kind === 'cpu' ? 1 : 1048576);
    const result = amount + direction * delta;
    if (!Number.isSafeInteger(result) || result < 0) return null;
    if (kind === 'cpu') return `${result}m`;
    if (result % 1048576 === 0) return `${result / 1048576}Mi`;
    if (result % 1024 === 0) return `${result / 1024}Ki`;
    return String(result);
  }
  return {next};
})();
