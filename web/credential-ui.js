const outcomeNames = {active:'Active credential', matching:'Document matches', revoked:'Credential revoked', altered:'Document mismatch', unknown:'Credential not found', unavailable:'Status unavailable', legacy:'Historical record'};
async function shareCredential(id) {
    try {
        const response = await fetch('/credentials/' + id + '/share',{method:'POST'});
        const data = await response.json();
        if (!response.ok) throw Error(data.error);
        const link = new URL(data.path, location.origin).href;
        const qr = new QRCode(0,1);
        qr.addData(link); qr.make();
        const dialog = document.createElement('dialog');
        dialog.style.maxWidth = 'min(90vw, 28rem)';
        dialog.innerHTML = '<h2>Share credential</h2><canvas aria-label="QR code for the credential"></canvas><p></p><input aria-label="Share link" readonly style="width:100%"><button>Close</button>';
        const canvas = dialog.querySelector('canvas');
        const n = qr.getModuleCount(), scale = 5;
        canvas.width = canvas.height = (n+8)*scale; canvas.style.maxWidth='100%';
        const ctx=canvas.getContext('2d');ctx.fillStyle='white';ctx.fillRect(0,0,canvas.width,canvas.height);ctx.fillStyle='black';
        for(let row=0;row<n;row++) for(let col=0;col<n;col++) if(qr.isDark(row,col)) ctx.fillRect((col+4)*scale,(row+4)*scale,scale,scale);
        dialog.querySelector('p').textContent='Anyone with this link can view the evidence until '+new Date(data.expires).toLocaleString()+'.';
        dialog.querySelector('input').value=link;dialog.querySelector('input').onclick=e=>e.target.select();
        dialog.querySelector('button').onclick=()=>dialog.close();dialog.onclose=()=>dialog.remove();
        document.body.append(dialog);dialog.showModal();
    } catch(e){alert(e.message);}
}
async function scanPresentation() {
    if (!window.BarcodeDetector || !navigator.mediaDevices) {alert('Use your phone’s camera app to open the holder’s QR code, or open their share link.');return;}
    const dialog=document.createElement('dialog');
    dialog.innerHTML='<video autoplay playsinline muted style="max-width:70vw"></video><p>Point the camera at a credential QR code.</p><button>Cancel</button>';
    document.body.append(dialog);dialog.showModal();
    let stream;const stop=()=>stream?.getTracks().forEach(t=>t.stop());
    dialog.querySelector('button').onclick=()=>dialog.close();dialog.onclose=()=>{stop();dialog.remove();};
    try{
        const detector=new BarcodeDetector({formats:['qr_code']});
        stream=await navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}});
        if(!dialog.open){stop();return;}
        const video=dialog.querySelector('video');video.srcObject=stream;await video.play();
        while(dialog.open){
            const results=await detector.detect(video);
            for(const code of results){
                let url;try{url=new URL(code.rawValue);}catch{continue;}
                if(url.origin===location.origin && url.pathname==='/presentation' && url.hash){dialog.close();location.assign(url.href);return;}
            }
            await new Promise(resolve=>setTimeout(resolve,150));
        }
    }catch(e){if(dialog.open)dialog.close();alert('Camera unavailable. Open the holder’s share link directly.');}finally{stop();}
}
