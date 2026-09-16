const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');
const stepper = vm.runInNewContext(readFileSync(`${__dirname}/static/resource-stepper.js`, 'utf8') + '\nresourceStepper;');
test('CPU stepping uses exact millicores', () => {
  assert.equal(stepper.next('cpu', '0.5', 10, 1), '510m');
  assert.equal(stepper.next('cpu', '100m', 50, -1), '50m');
  assert.equal(stepper.next('cpu', '', 10, 1), '10m');
  assert.equal(stepper.next('cpu', '1m', 10, -1), null);
  assert.equal(stepper.next('cpu', '0.0001', 1, 1), null);
});
test('memory stepping converts binary units without rounding', () => {
  assert.equal(stepper.next('memory', '1Gi', 64, 1), '1088Mi');
  assert.equal(stepper.next('memory', '1.5Gi', 64, -1), '1472Mi');
  assert.equal(stepper.next('memory', '512Ki', 1, 1), '1536Ki');
  assert.equal(stepper.next('memory', '1048576', 1, 1), '2Mi');
});
test('templates, invalid values and overflow are never rewritten', () => {
  for (const value of ['{{var:CPU}}','{{env:MEMORY}}','{{NAME}}','1000ABC','-1','1e3','NaN']) {
    assert.equal(stepper.next('cpu', value, 10, 1), null);
    assert.equal(stepper.next('memory', value, 64, 1), null);
  }
  assert.equal(stepper.next('memory', '1G', 64, 1), null);
  assert.equal(stepper.next('cpu', '9007199254740991m', 10, 1), null);
});
