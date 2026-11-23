'use strict';
(() => {
    const content = select('#hiring-content');
    const message = select('#hiring-message');
    let revision = 0;
    const url = (id, action) => '/career/jobs/' + encodeURIComponent(id) + '/' + action;
    const jobHash = id => '#hiring/opening/' + encodeURIComponent(id);
    function link(label, href, style='btn ghost') {
        const a = node('a', label, style);
        a.href = href;
        return a;
    }
    function openingList(jobs) {
        content.innerHTML = '<div class="bar"><h1>Hiring</h1><a class="btn" href="#hiring/new">Post an opening</a></div><section class="card"><h2>Your openings</h2><label>Search by role or company<input type="search"></label><div class="list"></div></section>';
        const input = content.querySelector('input');
        const list = content.querySelector('.list');
        function filter() {
            const query = input.value.trim().toLowerCase();
            const rows = jobs.slice().reverse().filter(j => [j.title,j.company].join(' ').toLowerCase().includes(query)).map(j => {
                const row = node('article','','item bar');
                const text = node('div','');
                text.append(node('h3',j.title),node('small',[j.company,j.location,j.type].filter(Boolean).join(' · ')));
                row.append(text,link('Review applicants',jobHash(j.id)));
                return row;
            });
            list.replaceChildren(...(rows.length ? rows : [node('p',jobs.length ? 'No matching openings.' : 'Post your first opening to start receiving applications.')]));
        }
        input.oninput = filter;
        filter();
    }
    function postForm(data) {
        content.innerHTML = '<h1>Post an opening</h1><form id="post" class="card"><div class="grid"><label>Company<input required name="company"></label><label>Job title<input required name="title"></label><label>Location<input required name="location"></label><label>Employment type<input required name="type" placeholder="Internship, full-time…"></label></div><label>Job description<textarea required name="description"></textarea></label><label>Required skills, separated by commas<input required name="requirements"></label><label>Interview stages and expectations<textarea name="interview"></textarea></label><div class="actions"><button class="btn">Publish opening</button><a href="#hiring" class="btn ghost">Cancel</a></div></form>';
        const company = content.querySelector('[name=company]');
        if (data.org && data.org !== '-') { company.value=data.org; company.readOnly=true; }
    }
    function applicant(c) {
        const card = node('article','','card');
        const heading = node('div','','bar');
        heading.append(node('h2',c.Student),node('span',c.Status,'badge'));
        card.append(heading,node('p','Verified requirements · '+(c.Verified.join(', ') || 'None yet')),node('p','Awaiting evidence · '+(c.Missing.join(', ') || 'None')));
        for (const evidence of c.Evidence || []) {
            const output = node('div','','item');
            output.setAttribute('role','status');
            const button = node('button','Verify credential from '+evidence.Issuer,'btn ghost');
            button.onclick = async () => {
                button.disabled=true;
                output.textContent='Checking the ledger…';
                try { await showVerification(evidence.ID,'',output); }
                catch(error) { output.textContent=error.message; }
                finally { button.disabled=false; }
            };
            card.append(button,output);
        }
        const form=node('form','');
        form.className='invite';
        form.dataset.student=c.Student;
        form.innerHTML='<label>Interview instructions<textarea required maxlength="1000" name="note"></textarea></label>';
        form.elements.note.value=c.Note || '';
        form.append(node('button',c.Note ? 'Update invitation' : 'Invite to interview','btn'));
        card.append(form);
        return card;
    }
    async function render() {
        const version=++revision;
        const hash=location.hash;
        const hiring=hash==='#hiring' || hash.startsWith('#hiring/');
        select('#verification').hidden=hiring;
        select('#hiring').hidden=!hiring;
        document.querySelectorAll('nav a').forEach(a=>{
            if(a.hash===(hiring?'#hiring':'#verification')) a.setAttribute('aria-current','page');
            else a.removeAttribute('aria-current');
        });
        message.textContent='';
        if (!hiring) return;
        content.textContent='Loading openings…';
        try {
            const data=await read('/career/data');
            if(version!==revision)return;
            if(hash==='#hiring')return openingList(data.jobs);
            if(hash==='#hiring/new')return postForm(data);
            const job=data.jobs.find(j=>jobHash(j.id)===hash);
            if(!job)throw Error('This opening is unavailable.');
            const applicants=await read(url(job.id,'applicants'));
            if(version!==revision)return;
            content.replaceChildren(link('← Your openings','#hiring'),node('h1',job.title),node('p',[job.company,job.location].filter(Boolean).join(' · ')),node('h2','Applicants · '+applicants.length));
            const list=node('div','','cards');
            list.dataset.job=job.id;
            list.append(...applicants.map(applicant));
            if(!applicants.length)list.append(node('p','Applications will appear here when students apply.'));
            content.append(list);
        } catch(error) {
            if(version===revision){content.replaceChildren(link('Your openings','#hiring'));message.textContent=error.message;const retry=node('button','Retry','btn ghost');retry.onclick=render;content.append(retry);}
        }
    }
    content.onsubmit=async e=>{
        e.preventDefault();
        const f=e.target, version=revision, button=f.querySelector('button');
        const fields=Object.fromEntries(new FormData(f));
        const posting=f.id==='post';
        if(posting){
            fields.requirements=fields.requirements.split(',').map(s=>s.trim()).filter(Boolean);
            if(!fields.requirements.length){message.textContent='Enter at least one required skill.';return;}
        }
        button.disabled=true;
        try {
            const result=await read(posting?'/jobs':url(f.closest('[data-job]').dataset.job,'invite'),{
                method:'POST',headers:{'Content-Type':'application/json'},
                body:JSON.stringify(posting?fields:{Student:f.dataset.student,Note:fields.note})
            });
            if(version!==revision)return;
            if(posting)location.hash=jobHash(result.job.id);
            else {await render();message.textContent='Interview instructions saved.';}
        }catch(error){if(version===revision)message.textContent=error.message;}
        finally{button.disabled=false;}
    };
    window.addEventListener('hashchange',render);
    render();
})();
