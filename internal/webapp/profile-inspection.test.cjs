const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');
const source = readFileSync(`${__dirname}/static/ui.js`, 'utf8');
function setup(fail) {
 const elements = {'#defaults-overview': {innerHTML:''}, '#defaults-status': {innerHTML:''}};
 const context = vm.createContext({
  document:{body:{dataset:{}},addEventListener(){},querySelector:s=>elements[s]},
  profileEditor:{active:()=>true,render(){}},
  fetch:async()=>{if(fail)throw new Error('Inspection unavailable');return {ok:true,json:async()=>({namespace:'test',defaults:{model:{}}})};},
 });
 vm.runInContext(source,context);
 vm.runInContext("state.env='dev'; state.active='defaults'; renderApps=()=>{};",context);
 return {context,elements};
}
test('inspection refresh clears loading while a profile is open',async()=>{
 const {context,elements}=setup(false);
 await vm.runInContext('loadInspection(true)',context);
 assert.match(elements['#defaults-status'].innerHTML,/Source catalogs loaded/);
 assert.doesNotMatch(elements['#defaults-status'].innerHTML,/Loading/);
});
test('inspection failure replaces loading without closing the profile',async()=>{
 const {context,elements}=setup(true);
 await vm.runInContext('loadInspection(true)',context);
 assert.match(elements['#defaults-status'].innerHTML,/Inspection unavailable/);
 assert.doesNotMatch(elements['#defaults-status'].innerHTML,/Loading/);
});
