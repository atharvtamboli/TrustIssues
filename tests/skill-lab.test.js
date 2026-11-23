'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const context = vm.createContext({});
vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/skill-lab.js'),'utf8'),context);
const role = (id, gaps, owner='employer', requirements=gaps) => ({
    job:{id,title:id,owner,requirements}, missing_skills:gaps
});
test('verified evidence, simulations, practice openings and unlisted roles stay distinct', () => {
    const matches = [role('ready',[], 'org',['Go']), role('single',['SQL','sql']),
        role('pair',['SQL','Python']), role('practice',['Python'],''),role('unlisted',[])];
    const initial = context.buildSkillScenario(matches, new Set(), false);
    assert.equal(initial.ready,1);
    assert.equal(initial.skills[0].gains,1);
    assert.equal(initial.skills[0].demand,2);
    assert.equal(initial.pair.gains,2);
    assert.equal(initial.unlisted,1);
    const result = context.buildSkillScenario(matches,new Set(['sql']),true);
    assert.equal(result.ready,1);
    assert.equal(result.projected,2);
    assert.equal(result.skills[0].gains,2);
    assert.equal(result.pair,null);
});
test('best pair agrees with exhaustive search across every subset of a four-skill catalog', () => {
    const keys=['a','b','c','d'];
    const subset=mask=>keys.filter((_,i)=>mask & (1<<i));
    for(let seed=1;seed<=24;seed++) {
        const matches=[];
        for(let mask=1;mask<16;mask++) {
            for(let n=0;n<(mask*seed+mask*mask)%5;n++) matches.push(role(String(mask)+'-'+n,subset(mask)));
        }
        for(let mask=0;mask<16;mask++) {
            const selected=new Set(subset(mask));
            const result=context.buildSkillScenario(matches,selected,false);
            const gains=extra=>matches.filter(m=>m.missing_skills.every(s=>selected.has(s)||extra.includes(s))).length-result.projected;
            const singles=Math.max(0,...keys.map(k=>gains([k])));
            const best=Math.max(0,...keys.flatMap(k=>keys.map(v=>gains([k,v]))));
            assert.equal(result.skills[0]?.gains||0,singles);
            assert.equal(result.pair?.gains||singles,best);
            if(result.pair) assert.equal(gains(result.pair.keys),best);
        }
    }
});
