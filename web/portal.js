'use strict';
const select = selector => document.querySelector(selector);
const notice = select('#notice');
function node(tag, text, style) {
    const element = document.createElement(tag);
    element.textContent = text;
    if (style) element.className = style;
    return element;
}
function setTheme(theme) {
    document.documentElement.dataset.theme = theme;
    select('#theme').setAttribute('aria-pressed', String(theme === 'dark'));
}
try { setTheme(localStorage.getItem('fatbot-theme') || 'light'); } catch { setTheme('light'); }
select('#theme').onclick = () => {
    const theme = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    setTheme(theme);
    try { localStorage.setItem('fatbot-theme', theme); } catch {}
};
async function read(path, options) {
    const response = await fetch(path, options);
    if (response.status === 401) { location.assign('/login'); throw Error('Please sign in again.'); }
    if (response.status === 204) return null;
    const data = await response.json();
    if (!response.ok && !data.outcome) throw Error(data.error || 'Request failed. Please retry.');
    return data;
}
function action(label, run) {
    const button = node('button', label, 'btn');
    button.onclick = async () => {
        button.disabled = true;
        notice.textContent = '';
        try { await run(); } catch (error) { notice.textContent = error.message; }
        finally { button.disabled = false; }
    };
    return button;
}
function record(item, pending) {
    const row = node('article', '', 'item');
    row.append(node('h3',item.file_name),node('p',[item.holder_id, ...(item.skills || [])].filter(Boolean).join(' · ')));
    const controls = node('div','','actions');
    if (pending) {
        const file = node('a','Download evidence','btn ghost');
        file.href = '/documents/'+encodeURIComponent(item.id);
        controls.append(file,action(item.status==='issuing'?'Retry confirmation':'Approve', async () => {
            await read('/approve',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:item.id})});
            await refresh();
        }));
    } else {
        controls.append(node('span',item.status,'badge'));
        if (item.status !== 'revoked') controls.append(action(item.status==='revoking'?'Retry revocation':'Revoke',async () => {
            if (!confirm('Revoke this credential? This action is permanent.')) return;
            await read('/credentials/'+encodeURIComponent(item.credential_id)+'/revoke',{method:'POST'});
            await refresh();
        }));
    }
    row.append(controls);
    return row;
}
let version = 0;
async function refresh() {
    const current = ++version;
    try {
        const [pending, blocks] = await Promise.all([read('/pending-files'),read('/blocks')]);
        if (version !== current) return;
        const issued = blocks.filter(b=>b.index!==0).reverse();
        select('#stats').replaceChildren(...[pending.length+' pending',issued.length+' issued',issued.filter(b=>!['revoked','revoking'].includes(b.status)).length+' active'].map(t=>node('span',t,'badge')));
        for (const [target,items,isPending] of [['#queue',pending,true],['#history',issued,false]]) {
            select(target).replaceChildren(...(items.length ? items.map(i=>record(i,isPending)) : [node('p',isPending?'No submissions awaiting review.':'No credentials issued yet.')]));
        }
        notice.textContent='';
    } catch (error) { if(version===current) notice.textContent=error.message; }
}
async function verify(form) {
    const fields = new FormData(form);
    let ref = fields.get('ref').trim(), query = '';
    if (form.id === 'compare') {
        const digest = await crypto.subtle.digest('SHA-256',await fields.get('file').arrayBuffer());
        const hex = Array.from(new Uint8Array(digest), byte=>byte.toString(16).padStart(2,'0')).join('');
        ref ||= hex;
        query = '?digest='+hex;
    }
    if (!ref) throw Error('Enter a credential reference.');
    await showVerification(ref,query,select('#result'));
}
async function showVerification(ref, query, pane) {
    const result = await read('/verify/hash/'+encodeURIComponent(ref)+query);
    pane.replaceChildren(node('h2',outcomeNames[result.outcome] || 'Verification result'),node('p',result.message || ''));
    for (const [label,value] of [['Ledger',result.ledger],['Issuer',result.issuer_org],['Transaction',result.transaction_id],['Checked',result.checked_at]]) {
        if(value) pane.append(node('p',label+' · '+value));
    }
    pane.hidden=false;
    pane.scrollIntoView({block:'nearest'});
}
if (document.body.dataset.role === 'issuer') {
    select('#refresh').onclick = refresh;
    refresh();
} else {
    select('#scan').onclick=scanPresentation;
    select('#presentation').onsubmit = event => {
        event.preventDefault();
        const url=new URL(new FormData(event.target).get('link'));
        if(url.origin===location.origin && url.pathname==='/presentation' && url.hash.length>1) location.assign(url.href);
        else notice.textContent='Enter a holder presentation link from this TrustIssues installation.';
    };
    const forms=[select('#lookup'),select('#compare')];
    for (const form of forms) form.onsubmit=async event=>{
        event.preventDefault();
        forms.forEach(f=>f.querySelector('button').disabled=true);
        select('#result').hidden=true;
        notice.textContent='Checking credential…';
        try { await verify(form); notice.textContent=''; } catch(error){notice.textContent=error.message;}
        finally {forms.forEach(f=>f.querySelector('button').disabled=false);}
    };
}
