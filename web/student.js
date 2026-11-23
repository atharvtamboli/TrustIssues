'use strict';
const $ = selector => document.querySelector(selector);
const content = $('#content');
const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const link = (view, title, primary=false) => `<a class="btn ${primary?'':'ghost'}" href="#${view}">${title}</a>`;
const chip = text => `<span class="badge">${escape(text)}</span>`;
const external = (url, title) => `<a class="btn ghost" target="_blank" rel="noopener noreferrer" href="${escape(url)}">${escape(title)} ↗</a>`;
const title = (name, description='') => `<h1>${escape(name)}</h1><p>${escape(description)}</p>`;
const sections = {planner:'Skill planner',overview:'Overview',credentials:'Blockchain credentials',jobs:'Job openings',eligibility:'Eligibility checker',roadmap:'Career roadmap',learn:'Upskill & grow',opportunities:'Unstop opportunities',assessment:'Skill assessment',coach:'Career coach'};
let data, selected, generation=0;
const route = () => Object.hasOwn(sections,location.hash.slice(1)) ? location.hash.slice(1) : 'credentials';
const jobPath = action => '/career/jobs/'+encodeURIComponent(selected)+'/'+action;
async function request(path, body) {
    const options = body instanceof FormData ? {method:'POST',body} : body === undefined ? {} : {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)};
    const response = await fetch(path, options);
    if (response.status===401) { location.assign('/login'); throw Error('Sign in to continue.'); }
    if (!response.ok) throw Error((await response.json().catch(()=>({}))).error || 'Could not load this information. Please retry.');
    return response.status===204 ? null : response.json();
}
function jobPicker() {
    return `<label>Target opening<select id="target">${data.jobs.map(j=>`<option value="${escape(j.id)}" ${j.id===selected?'selected':''}>${escape(j.title)} · ${escape(j.company || 'Practice opening')}</option>`).join('')}</select></label>`;
}
function ring(percent) {
    return `<div class="ring" style="--p:${percent}"><b>${percent}%</b></div>`;
}
function tile(view,icon,copy) {
    return `<a class="tile card" href="#${view}"><i aria-hidden="true">${icon}</i><strong>${sections[view]}</strong><span>${copy}</span></a>`;
}
function overview(files) {
    const issued=(files.blockchain||[]).filter(b=>b.index!==0 && !['revoked','revoking'].includes(b.status)).length;
    const pending=(files.pending||[]).length;
    const invitations=Object.values(data.statuses).filter(s=>s==='Interview invited').length;
    const target=data.jobs.find(j=>j.id===selected);
    return title('Your credential portal', 'Manage your evidence and prepare for your next role.')+
        `<div class="card feature bar"><div><small style="color:inherit">Career roadmap</small><h2>${escape(target?.title || 'Choose your next role')}</h2><p>${escape(target?.company || 'Explore the job board and build a preparation plan.')}</p></div>${link(target?'roadmap':'jobs',target?'Open roadmap →':'Explore openings →',true)}</div>
        <section class="card summary bar"><div><h2>Credential wallet</h2><p>${issued} issued · ${pending} pending review · ${invitations} interview invitations</p></div><button class="btn" data-upload>Upload certificate</button></section>
        <div class="grid">${tile('credentials','◈','Upload a certificate, track approval and share verified evidence.')}${tile('eligibility','◎','Compare your verified skills with an opening’s requirements.')}${tile('jobs','▦','Browse openings, apply and read interview invitations.')}${tile('learn','↗','Find courses and practice tasks for your target role.')}${tile('opportunities','✦','Explore hackathons, workshops and competitions on Unstop.')}${tile('assessment','✓','Test your fundamentals and review the answers.')}${tile('coach','✧','Ask about skill gaps, courses or interview preparation.')}</div>`;
}
function jobRows(search='') {
    const jobs=data.jobs.slice().reverse().filter(j=>[j.company,j.title,j.location].join(' ').toLowerCase().includes(search.trim().toLowerCase()));
    return jobs.map(j=>`<article class="item bar"><div><small>${escape(j.company||'Practice opening')}</small><h3>${escape(j.title)}</h3><p>${escape([j.location,j.type].filter(Boolean).join(' · '))}</p>${data.statuses[j.id]?chip(data.statuses[j.id]):''}</div><button class="btn ghost" data-job="${escape(j.id)}">View roadmap →</button></article>`).join('')||'<p>No matching openings. Try another search or check back after an employer posts a role.</p>';
}
function credentials(files) {
    const pending = files.pending || [], issued = (files.blockchain||[]).filter(b=>b.index!==0);
    const cards = [...pending.map(b=>({...b,pending:true})),...issued.slice().reverse()].map(b=>{
        const active=!b.pending && !['revoked','revoking'].includes(b.status);
        return `<article class="item"><div class="bar"><h3>${escape(b.file_name)}</h3>${chip(b.pending?'Pending approval':active?'Issued':b.status)}</div><p>${escape(b.issuer_org)} · ${escape((b.skills||[]).join(', ')||'No skills listed')}</p>${b.credential_id?`<details><summary>Credential details</summary><p>ID · ${escape(b.credential_id)}</p><p>Issuance reference · ${escape(b.transaction_id||b.hash)}</p></details>`:''}${active?`<div class="actions"><a class="btn ghost" href="/my-evidence/${encodeURIComponent(b.credential_id)}">Download evidence</a><button class="btn share" data-share="${escape(b.credential_id)}">Share</button></div>`:''}</article>`;
    }).join('');
    return title('Blockchain credentials','Track certificates from submission through issuance.')+`<div class="card"><div class="bar"><h2>${issued.length+pending.length} certificates · ${pending.length} pending</h2><div class="actions"><button class="btn ghost" data-refresh>Refresh</button><button class="btn" data-upload>Upload certificate</button></div></div>${cards||'<p>Your first certificate will appear here after upload.</p>'}</div>`;
}
const resources = [
    ['National-level hackathons','Build with a team and compete.','https://unstop.com/hackathons'],
    ['Workshops','Explore hands-on learning and career workshops.','https://unstop.com/workshops-webinars'],
    ['Competitions','Find challenges across technical and business skills.','https://unstop.com/competitions']
];
const quiz = [
    ['SQL','Which clause filters individual rows before grouping?', ['ORDER BY','WHERE','HAVING'],1,'WHERE filters rows before GROUP BY. HAVING filters groups.'],
    ['Web development','Which element provides a native keyboard-accessible action?', ['button','span','div'],0,'A button supports focus and keyboard activation by default.'],
    ['Go','What does a deferred call normally do?', ['Runs on a new goroutine','Runs before each loop iteration','Runs when the surrounding function returns'],2,'Deferred calls run when their surrounding function returns, in reverse order.'],
    ['Credential verification','What can matching file hashes help confirm?', ['Who wrote the document','That the file bytes match','That the credential is still active'],1,'A digest compares file contents. Revocation status requires a separate check.']
];
function assessment() {
    return title('Skill assessment','Four practice questions on technical fundamentals. Your score is private practice feedback.')+`<form class="card" id="quiz">${quiz.map(([skill,q,answers],i)=>`<fieldset><legend>${skill} · ${q}</legend>${answers.map((a,n)=>`<label><input type="radio" name="q${i}" value="${n}" required>${a}</label>`).join('')}</fieldset>`).join('')}<button class="btn">Check answers</button><div id="score" aria-live="polite"></div></form>`;
}
function journey(view,plan) {
    const steps=plan.Steps, matched=plan.Matched, gaps=steps.filter(s=>!s.Verified);
    const percent=steps.length ? Math.round(100*matched/steps.length) : 0;
    const header = title(sections[view],plan.Job.description)+jobPicker();
    const progress=`<div class="card row">${ring(percent)}<div><h2>Verified skill match</h2><p>${matched} of ${steps.length} listed requirements have active evidence.</p></div></div>`;
    if(view==='eligibility') return header+progress+`<div class="grid">${[[true,'Verified requirements'],[false,'Evidence needed']].map(([v,name])=>`<section class="card"><h2>${name}</h2><div class="chips">${steps.filter(s=>s.Verified===v).map(s=>chip(s.Skill)).join('')||'None listed'}</div></section>`).join('')}</div>${link('roadmap','Open preparation roadmap →',true)}`;
    if(view==='coach') return header+`<form class="card" id="ask"><h2>${data.ai?'AI career coach':'Roadmap guide'}</h2><p>${data.ai?'Submitting sends your question, the selected opening and relevant progress to the configured AI provider.':'Local guidance uses this role’s requirements and interview stages.'}</p><div class="actions">${['What skills am I missing?','Which course first?','How should I prepare for the interview?'].map(q=>`<button class="btn ghost" type="button" data-prompt="${q}">${q}</button>`).join('')}</div><label>Your question<textarea required maxlength="1000" name="question"></textarea></label><button class="btn" type="submit">Ask coach</button><p class="reply" role="status"></p></form>`;
    const task=s=>`<article class="item"><h3>${escape(s.Skill)}</h3><p>${escape(s.Practice)}</p>${external(s.Course.URL,s.Course.Title)} <button class="btn ghost" data-skill="${escape(s.Skill)}" data-done="${!s.Done}">${s.Done?'Reopen practice':'Mark practiced'}</button></article>`;
    if(view==='learn')return header+`<section class="card"><h2>Courses for your skill gaps</h2>${gaps.map(task).join('')||'<p>No outstanding skill gaps for this opening.</p>'}</section>`;
    const app=plan.Application;
    return header+progress+`<div class="stages">${[['Learn & practice',gaps.filter(s=>!s.Done)],['Practiced',gaps.filter(s=>s.Done)],['Credential verified',steps.filter(s=>s.Verified)]].map(([n,list])=>`<section class="card"><h2>${n}</h2>${list.map(s=>s.Verified?chip(s.Skill):task(s)).join('')||'<p>No skills in this stage.</p>'}</section>`).join('')}</div>
    <section class="card feature"><h2>Apply &amp; interview</h2><p>${escape(plan.Job.interview||'Interview stages have not been posted.')}</p>${app?`<strong>${escape(app.Status)}</strong><p>${escape(app.Note)}</p>`:plan.Job.owner?'<p>Applying shares your username, matching skills and credential references with the posting employer.</p><button class="btn" data-apply>Apply with verified skills</button>':'<p>This archived opening is available for practice.</p>'} ${link('coach','Ask career coach')}</section>`;
}
async function render() {
    const seq=++generation, view=route();
    $('#section').textContent=sections[view]; $('#notice').textContent='';
    document.querySelectorAll('nav a').forEach(a=>{if(a.hash==='#'+view)a.setAttribute('aria-current','page');else a.removeAttribute('aria-current')});
    content.innerHTML='<p role="status">Loading…</p>';
    try {
        const snapshot=await request('/career/data');
        if(seq!==generation)return;
        data=snapshot;
        selected=data.target || data.jobs[0]?.id;
        $('.account').textContent=data.user;
        let html;
        if(view==='overview')html=overview(await request('/all-files'));
        else if(view==='credentials')html=credentials(await request('/all-files'));
        else if(view==='planner'){
            const dashboard=await request('/student/dashboard');
            if(seq!==generation)return;
            html=skillLab(dashboard.job_matches);
        }
        else if(view==='jobs')html=title('Explore openings','Search by company, location or role.')+`<div class="card"><label>Search openings<input id="search" type="search"></label><div id="jobs">${jobRows()}</div></div>`;
        else if(view==='assessment')html=assessment();
        else if(view==='opportunities')html=title(sections[view],'Browse current listings directly on Unstop.')+`<div class="grid">${resources.map(([t,p,url])=>`<section class="card"><h2>${t}</h2><p>${p}</p>${external(url,'Explore on Unstop')}</section>`).join('')}</div>`;
        else if(view==='learn' && !selected)html=title(sections.learn,'Start with a foundation course. Role-specific recommendations appear when a company posts an opening.')+`<div class="grid">${[['Go','A Tour of Go','https://go.dev/tour/'],['SQL','SQLBolt interactive lessons','https://sqlbolt.com/'],['Python','CS50 Python','https://cs50.harvard.edu/python/']].map(([name,label,url])=>`<article class="card"><h2>${name}</h2>${external(url,label)}</article>`).join('')}</div>`;
        else if(!selected)html=title(sections[view],'No openings have been posted yet. Choose a role here once an employer publishes one.')+link('jobs','View job board')+' '+link('assessment','Practice your skills');
        else html=journey(view,await request(jobPath('plan')));
        if(seq===generation)content.innerHTML=html;
    } catch(error) {
        if(seq===generation){content.innerHTML='<button class="btn" data-refresh>Retry</button>';$('#notice').textContent=error.message;}
    }
}
async function selectJob(id,view) {
    await request('/career/jobs/'+encodeURIComponent(id)+'/plan',{});
    if(route()===view)await render();else location.hash=view;
}
async function openUpload(){
    const orgs=await request('/issuers'), form=$('#uploadForm');
    form.reset();
    form.elements.issuer_org.replaceChildren(new Option('Choose an institution',''),...orgs.map(o=>new Option(o,o)));
    form.querySelector('button').disabled=!orgs.length;
    $('#uploadStatus').textContent=orgs.length?'':'An issuer account must be added before you can submit a certificate.';
    $('#upload').showModal();
}
document.addEventListener('click',async e=>{
    const b=e.target.closest('button'); if(!b)return;
    try {
        if(route()==='planner' && changeLab(b))return;
        if(b.id==='cancel')return $('#upload').close();
        if(b.id==='theme'){const dark=document.documentElement.dataset.theme!=='dark';document.documentElement.dataset.theme=dark?'dark':'light';localStorage.setItem('fatbot-theme',document.documentElement.dataset.theme);return;}
        if(b.hasAttribute('data-refresh'))return await render();
        if(b.dataset.prompt){$('#ask textarea').value=b.dataset.prompt;$('#ask textarea').focus();return;}
        if(b.hasAttribute('data-upload'))return await openUpload();
        if(b.dataset.share)return await shareCredential(b.dataset.share);
        if(!b.dataset.job && !b.dataset.skill && !b.hasAttribute('data-apply'))return;
        b.disabled=true;
        if(b.dataset.job)await selectJob(b.dataset.job,'roadmap');
        else {if(b.dataset.skill)await request(jobPath('plan'),{Skill:b.dataset.skill,Done:b.dataset.done==='true'});else await request(jobPath('apply'),{});await render();}
    }catch(error){$('#notice').textContent=error.message;}finally{if(!b.closest('#uploadForm'))b.disabled=false;}
});
content.addEventListener('change',async e=>{
    if(e.target.id==='labPractice'){
        includePractice=e.target.checked;
        const available=buildSkillScenario(labMatches,new Set(),includePractice).skills;
        const keys=new Set(available.map(skill=>skill.key));
        for(const key of labSelected.keys())if(!keys.has(key))labSelected.delete(key);
        updateLab();
        return;
    }
    if(e.target.id!=='target')return;
    e.target.disabled=true;
    try{await selectJob(e.target.value,route());}catch(error){e.target.value=selected;e.target.disabled=false;$('#notice').textContent=error.message;}
});
content.addEventListener('input',e=>{if(e.target.id==='search')$('#jobs').innerHTML=jobRows(e.target.value);});
document.addEventListener('submit',async e=>{
    const f=e.target;
    if(!['ask','quiz','uploadForm'].includes(f.id))return;
    e.preventDefault();
    const fields=new FormData(f), submit=f.querySelector('button[type=submit]')||f.querySelector('button');
    submit.disabled=true;
    try{
        if(f.id==='ask'){const r=await request(jobPath('coach'),{Question:fields.get('question')});f.querySelector('.reply').textContent=r.mode+'\n'+r.answer;}
        if(f.id==='uploadForm'){
            if(fields.get('file').size>16*1024*1024)throw Error('Select a file up to 16 MiB.');
            await request('/upload',fields);$('#upload').close();await render();$('#notice').textContent='Certificate submitted for issuer review.';
        }
        if(f.id==='quiz'){
            let total=0;
            const review=quiz.map(([skill,q,answers,correct,why],i)=>{const ok=Number(fields.get('q'+i))===correct;if(ok)total++;return `<p><strong>${skill} · ${ok?'Correct':'Review'}</strong><br>${why}</p>`;});
            $('#score').innerHTML=`<h2>Your result · ${total} / ${quiz.length}</h2>${review.join('')}`;
        }
    }catch(error){(f.id==='uploadForm'?$('#uploadStatus'):$('#notice')).textContent=error.message;}finally{submit.disabled=false;}
});
try{document.documentElement.dataset.theme=localStorage.getItem('fatbot-theme')||'light';}catch{}
window.addEventListener('hashchange',()=>{render();window.scrollTo(0,0);});
render();
