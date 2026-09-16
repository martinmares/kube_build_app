const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');

const context = vm.createContext({});
vm.runInContext(readFileSync(`${__dirname}/static/template-input.js`, 'utf8'), context);

test('template display parser preserves compound source parts', () => {
  const source = '{{TSM_REGISTRY_URL}}/{{var:APP_NAME}}:{{env:TSM_RELEASE_ID}}';
  const parts = context.templateInput.tokenize(source);
  assert.deepEqual(JSON.parse(JSON.stringify(parts)), [
    {type:'passthrough', name:'TSM_REGISTRY_URL', value:'{{TSM_REGISTRY_URL}}'},
    {type:'text', value:'/'},
    {type:'var', name:'APP_NAME', value:'{{var:APP_NAME}}'},
    {type:'text', value:':'},
    {type:'env', name:'TSM_RELEASE_ID', value:'{{env:TSM_RELEASE_ID}}'},
  ]);
  assert.equal(parts.map(part => part.value).join(''), source);
});

test('raw values are not classified as templates', () => {
  assert.equal(context.templateInput.hasTemplate('80'), false);
  assert.equal(context.templateInput.hasTemplate('registry.example/app:tag'), false);
  assert.equal(context.templateInput.hasTemplate('{{var:PORT}}'), true);
});
