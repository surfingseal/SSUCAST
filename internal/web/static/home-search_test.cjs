'use strict';
const assert=require('node:assert/strict'),s=require('./home-search.js');
assert.deepEqual(s.cleanHistory(null),[]);
assert.deepEqual(s.cleanHistory([' 장학금 ',null,5,'','장학금','인턴']),['장학금','인턴']);
assert.equal(s.cleanHistory(['x'.repeat(101)]).length,0);
assert.deepEqual(s.remember(['장학금','인턴'],'인턴'),['인턴','장학금']);
assert.equal(s.remember(['a','b','c','d','e'],'f').length,5);
assert.deepEqual(s.remember(['장학금'],'  '),['장학금']);
console.log('SEARCH HISTORY NORMALIZATION AND LIMIT TESTS PASSED');
