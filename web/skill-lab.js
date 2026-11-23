'use strict';

function buildSkillScenario(matches, selected, practice) {
    const roles = [];
    const impact = new Map();
    const pairGains = new Map();
    let ready = 0, projected = 0, unlisted = 0;
    for (const match of matches) {
        const job = match.job;
        if (!job.owner && !practice) continue;
        if (!job.requirements?.length) { unlisted++; continue; }
        const gaps = new Map();
        for (const label of match.missing_skills || []) {
            gaps.set(label.trim().toLowerCase(), label.trim());
        }
        const remaining = [...gaps.keys()].filter(key => !selected.has(key)).sort();
        if (!gaps.size) ready++;
        if (!remaining.length) projected++;
        for (const key of remaining) {
            const skill = impact.get(key) || {key, label:gaps.get(key), gains:0, demand:0};
            skill.demand++;
            if (remaining.length === 1) skill.gains++;
            impact.set(key, skill);
        }
        if (remaining.length === 2) {
            const pair = JSON.stringify(remaining);
            pairGains.set(pair, (pairGains.get(pair) || 0) + 1);
        }
        roles.push({job, gaps, remaining});
    }
    const skills = [...impact.values()].sort((a,b) =>
        b.gains-a.gains || b.demand-a.demand || a.label.localeCompare(b.label));
    // Pairs without a shared two-gap role can only gain their two individual totals.
    if (skills.length >= 2) {
        const top = JSON.stringify([skills[0].key, skills[1].key].sort());
        if (!pairGains.has(top)) pairGains.set(top, 0);
    }
    let pair = null;
    for (const [id, shared] of pairGains) {
        const keys = JSON.parse(id);
        const gains = shared + keys.reduce((sum,key) => sum + impact.get(key).gains, 0);
        if (gains > (skills[0]?.gains || 0) &&
            (!pair || gains > pair.gains || (gains === pair.gains && id < pair.id))) {
            pair = {id, keys, gains};
        }
    }
    roles.sort((a,b) => a.remaining.length-b.remaining.length ||
        b.gaps.size-a.gaps.size || a.job.title.localeCompare(b.job.title));
    return {roles, ready, projected, skills, pair, unlisted};
}

let labMatches = [], labSelected = new Map(), includePractice = false;
function skillLab(matches) {
    labMatches = matches;
    labSelected = new Map();
    includePractice = !matches.some(m => m.job.owner);
    return title('Skill planner', 'Explore how learning one or two skills could change your match across openings.') +
        '<label class="lab-toggle"><input type="checkbox" id="labPractice" '+(includePractice?'checked':'')+'> Include archived practice openings</label><div id="labResults">'+labResults()+'</div>';
}
function labResults() {
    const {roles, ready, projected, skills, pair, unlisted} =
        buildSkillScenario(labMatches, labSelected, includePractice);
    const count = n => n === 1 ? '1 opening' : n+' openings';
    const skillButton = s => `<button class="btn ghost" data-lab-add="${escape(s.key)}">${escape(s.label)} <small>in ${count(s.demand)}</small></button>`;
    const selection = [...labSelected].map(([key,label]) =>
        `<button class="btn ghost" data-lab-remove="${escape(key)}" aria-label="Remove ${escape(label)}">${escape(label)} &times;</button>`).join('');
    const roleRows = roles.map(({job,gaps,remaining}) => {
        const status = !gaps.size ? 'Verified match' : !remaining.length ? 'Match in scenario' : remaining.length+' skills to cover';
        return `<article class="item bar"><div><small>${escape(job.company || 'Archived practice opening')}</small><h3>${escape(job.title)}</h3>${chip(status)}
            ${remaining.length ? '<p>Still needed &middot; '+remaining.map(key=>escape(gaps.get(key))).join(', ')+'</p>' : ''}</div>
            <button class="btn ghost" data-job="${escape(job.id)}">Open roadmap</button></article>`;
    }).join('');
    return `<section class="card feature"><h2>${count(projected)} with all listed skills covered</h2>
        <p>${count(ready)} with your current evidence &middot; ${projected-ready} additional in this scenario</p></section>
        <div class="lab-layout"><section class="card"><h2>Try a learning plan</h2>
        <p>Selections are private to this view. Employers see only your verified evidence.</p>
        ${selection ? '<h3>Selected skills</h3><div class="actions">'+selection+'<button class="btn ghost" data-lab-export>Download study brief</button><button class="btn ghost" data-lab-reset>Clear all</button></div>':''}
        ${skills.length ? `<h3>Next skill to try</h3><p><strong>${escape(skills[0].label)}</strong> ${skills[0].gains ? 'would cover the last requirement for '+count(skills[0].gains) : 'would move you closer to '+count(skills[0].demand)}.</p>` : ''}
        ${pair ? `<button class="btn" data-lab-pair="${escape(pair.id)}">Try ${pair.keys.map(key=>escape(skills.find(s=>s.key===key).label)).join(' + ')}</button><p>This pair would cover all remaining skills for ${count(pair.gains)}.</p>` : ''}
        ${skills.length ? '<h3>Add a skill</h3><div class="actions">'+skills.map(skillButton).join('')+'</div>' : ''}
        </section><section class="card"><h2>Opening breakdown</h2>${roleRows || '<p>No openings with listed skill requirements in this view.</p>'}
        ${unlisted ? '<p>'+count(unlisted)+' omitted because no skills are listed.</p>' : ''}</section></div>`;
}
function updateLab(focusKey) {
    const results = $('#labResults');
    results.innerHTML = labResults();
    $('#notice').textContent = 'Scenario updated. '+results.querySelector('h2').textContent+'.';
    const focus = focusKey != null && [...results.querySelectorAll('[data-lab-add],[data-lab-remove]')].find(b =>
        b.dataset.labAdd === focusKey || b.dataset.labRemove === focusKey);
    (focus || $('#labPractice')).focus({preventScroll:true});
}
function studyBrief() {
    const {roles,ready,projected} = buildSkillScenario(labMatches, labSelected, includePractice);
    const lines = ['FATBOT STUDY BRIEF', '', 'Current verified matches: '+ready,
        'Projected matches: '+projected, 'Includes archived practice openings: '+(includePractice?'Yes':'No'),
        '', 'Learning checklist'];
    for (const [key,label] of labSelected) {
        const affected = roles.filter(role => role.gaps.has(key));
        lines.push('[ ] '+label+' — required by '+affected.length+' opening(s)');
    }
    lines.push('', 'Roles affected by this plan');
    for (const {job,gaps,remaining} of roles) {
        if (gaps.size === remaining.length) continue;
        lines.push('', job.title+' | '+(job.company || 'Archived practice opening'),
            remaining.length ? 'Still needed: '+remaining.map(key=>gaps.get(key)).join(', ') : 'All listed requirements covered in this scenario.');
    }
    lines.push('', 'This is a learning projection. Verify new skills through your credential wallet.');
    return lines.join('\n');
}
function changeLab(button) {
    if (button.hasAttribute('data-lab-export')) {
        const url = URL.createObjectURL(new Blob([studyBrief()], {type:'text/plain;charset=utf-8'}));
        const download = document.createElement('a');
        download.href = url;
        download.download = 'fatbot-study-brief.txt';
        download.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        return true;
    }
    if (!button.matches('[data-lab-add],[data-lab-remove],[data-lab-pair],[data-lab-reset]')) return false;
    const {labAdd,labRemove,labPair} = button.dataset;
    if (labRemove !== undefined) labSelected.delete(labRemove);
    else if (labAdd !== undefined || labPair) {
        const keys = labPair ? JSON.parse(labPair) : [labAdd];
        const {skills} = buildSkillScenario(labMatches, labSelected, includePractice);
        for (const skill of skills) if (keys.includes(skill.key)) labSelected.set(skill.key, skill.label);
    } else labSelected.clear();
    updateLab(labAdd ?? labRemove ?? (labPair && JSON.parse(labPair)[0]));
    return true;
}
