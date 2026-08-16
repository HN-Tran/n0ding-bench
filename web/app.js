const $ = (s) => document.querySelector(s);
const state = { mode: '', runs: [], run: null, events: [], projection: null, view: 'latest' };

function safe(value) { const node = document.createElement('span'); node.textContent = String(value ?? '—'); return node.innerHTML; }
function shortTime(value) { if (!value) return '—'; return new Intl.DateTimeFormat(undefined,{hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value)); }
function shortDate(value) { if (!value) return '—'; return new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value)); }
async function api(path, options) { const response = await fetch(path, options); const body = await response.json().catch(()=>({})); if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`); return body; }
function announce(message, error=false) { $('#notice').textContent=message; $('#notice').classList.toggle('error',error); }

async function boot() {
  try {
    const health=await api('/healthz'); state.mode=health.mode;
    $('#connectionDot').className='signal ok'; $('#connectionText').textContent='Connected';
    const label=state.mode==='bench'?'Bench':'Dispatch';
    $('#modeEyebrow').textContent=`N0DING ${label.toUpperCase()}`;
    $('#evidenceKicker').textContent=state.mode==='bench'?'BENCHMARK EVIDENCE':'ROUTING EVIDENCE';
    $('#productCopy').textContent=state.mode==='bench'?'Compare case-level results from genuine benchmark events. Scores shown here are fixture evidence, not product claims.':'Inspect delegation, artifacts, approvals, failures, and retries only when the event log contains them.';
    await loadRuns(); announce(`${label} is connected. Start or select a run.`);
  } catch(error) { $('#connectionDot').className='signal bad'; $('#connectionText').textContent='Offline'; announce(`Cannot reach backend: ${error.message}`,true); }
}

async function loadRuns(selectId) {
  const payload=await api('/api/v1/runs'); state.runs=payload.runs||[];
  $('#runs').innerHTML=state.runs.length?state.runs.slice().reverse().map(run=>`<button class="run-button ${state.run?.id===run.id?'active':''}" data-id="${safe(run.id)}"><span class="run-name">${safe(run.name||run.id)}</span><span class="run-meta"><span>${safe(run.mode)}</span><time>${safe(shortDate(run.created_at))}</time></span></button>`).join(''):'<p class="empty">No runs yet.</p>';
  document.querySelectorAll('.run-button').forEach(button=>button.addEventListener('click',()=>selectRun(button.dataset.id)));
  const wanted=selectId||state.run?.id; if(wanted) await selectRun(wanted); else if(state.runs.length) await selectRun(state.runs[state.runs.length-1].id);
}

async function selectRun(id) {
  state.run=state.runs.find(run=>run.id===id)||{id};
  const [eventsPayload,projection]=await Promise.all([api(`/api/v1/runs/${encodeURIComponent(id)}/events`),api(`/api/v1/runs/${encodeURIComponent(id)}/projection`)]);
  state.events=eventsPayload.events||[]; state.projection=projection;
  document.querySelectorAll('.run-button').forEach(b=>b.classList.toggle('active',b.dataset.id===id));
  $('#replayRange').max=state.events.at(-1)?.sequence||0; $('#replayRange').value=$('#replayRange').max; $('#replayValue').value=$('#replayRange').value;
  render(); announce(`Showing ${state.events.length} stored events for ${state.run.name||id}.`);
}

function render() {
  const p=state.projection||{}; const status=p.status||'created';
  $('#status').textContent=status; $('#status').className=`status-${status}`; $('#steps').textContent=p.steps||0; $('#lastEvent').textContent=p.last_event_id||'—'; $('#created').textContent=shortDate(p.run?.created_at||state.run?.created_at); $('#runId').textContent=state.run?.id||'No run'; $('#evidenceHeading').textContent=state.run?.name||'Select a run';
  renderEvidence(); renderTimeline();
}

function visibleEvents(){if(state.view==='latest') return state.events; const upto=Number($('#replayRange').value); return state.events.filter(e=>e.sequence<=upto);}
function renderEvidence() {
  const events=visibleEvents(); if(!state.run){$('#domainEvidence').innerHTML='<p class="empty">Choose a run to inspect its evidence.</p>';return}
  if(state.mode==='bench') renderBench(events); else renderDispatch(events);
}
function renderBench(events){
  const results=events.filter(e=>e.type==='case.completed').map(e=>({id:e.sequence,...e.payload})); const completed=events.find(e=>e.type==='benchmark.completed');
	const attempts=events.filter(e=>e.type.startsWith('case.')).map(e=>({id:e.sequence,status:e.type.slice(5),...e.payload}));
  const models=[...new Set(results.map(r=>r.model).filter(Boolean))]; const avg=models.map(model=>{const xs=results.filter(r=>r.model===model).map(r=>Number(r.score));return {model,score:xs.reduce((a,b)=>a+b,0)/(xs.length||1),cases:xs.length}}).sort((a,b)=>b.score-a.score);
  const cards=`<div class="evidence-cards"><div class="evidence-card"><span>Models observed</span><strong>${models.length}</strong></div><div class="evidence-card"><span>Case events</span><strong>${attempts.length}</strong></div><div class="evidence-card"><span>Recorded winner</span><strong>${safe(completed?.payload?.winner||'Not recorded')}</strong></div></div>`;
  const rows=attempts.length?attempts.map(r=>`<tr><td>${safe(r.case)}</td><td>${safe(r.status)}</td><td>${safe(r.attempt)}</td><td>${safe(r.retry_of)}</td><td>${safe(r.scorer_version)}</td><td>${safe(r.score)}</td><td class="mono">#${r.id}</td></tr>`).join(''):'<tr><td colspan="7" class="empty">No case events in this view.</td></tr>';
  const comparison=avg.length?`<p class="section-label">Comparison derived from visible case.completed events</p><div class="artifact-list">${avg.map(x=>`<span class="artifact">${safe(x.model)} · ${Number.isFinite(x.score)?x.score.toFixed(3):'—'} avg · ${x.cases} case${x.cases===1?'':'s'}</span>`).join('')}</div>`:'';
  $('#domainEvidence').innerHTML=`${cards}${comparison}<p class="section-label">Attempts and outcomes</p><div class="table-wrap"><table><thead><tr><th>Case</th><th>Status</th><th>Attempt</th><th>Retry of</th><th>Scorer</th><th>Score</th><th>Source</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}
function renderDispatch(events){
  const delegated=events.filter(e=>e.type.includes('delegat')); const approvals=events.filter(e=>e.type.includes('approval')); const failures=events.filter(e=>e.type.endsWith('.failed')||e.type.includes('outcome_unknown')); const retries=events.filter(e=>e.type.includes('retry')); const artifacts=events.filter(e=>e.type==='artifact.created');
  const routes=delegated.length?delegated.map(e=>`<tr><td>${safe(e.payload.from||'—')}</td><td>${safe(e.payload.to||'—')}</td><td>${safe(e.payload.task||e.payload.reason||'—')}</td><td class="mono">#${e.sequence}</td></tr>`).join(''):'<tr><td colspan="4" class="empty">No delegation events in this view.</td></tr>';
  $('#domainEvidence').innerHTML=`<div class="evidence-cards"><div class="evidence-card"><span>Delegations</span><strong>${delegated.length}</strong></div><div class="evidence-card"><span>Approvals observed</span><strong>${approvals.length}</strong></div><div class="evidence-card"><span>Failure / unknown</span><strong>${failures.length}</strong></div><div class="evidence-card"><span>Retries observed</span><strong>${retries.length}</strong></div><div class="evidence-card"><span>Artifacts</span><strong>${artifacts.length}</strong></div></div><p class="section-label">Routing ledger</p><div class="table-wrap"><table><thead><tr><th>From</th><th>To</th><th>Task / reason</th><th>Source</th></tr></thead><tbody>${routes}</tbody></table></div>${artifacts.length?`<p class="section-label">Artifacts</p><div class="artifact-list">${artifacts.map(e=>`<span class="artifact">${safe(e.payload.name||'unnamed')} · #${e.sequence}</span>`).join('')}</div>`:''}`;
}
function renderTimeline(){
  const events=visibleEvents(); $('#timeline').innerHTML=events.length?events.map(e=>`<tr><td class="mono">${e.sequence}</td><td><time datetime="${safe(e.occurred_at)}">${safe(shortTime(e.occurred_at))}</time></td><td class="event-type">${safe(e.type)}</td><td><div class="payload">${Object.entries(e.payload||{}).map(([k,v])=>`<span class="datum" title="${safe(JSON.stringify(v))}">${safe(k)}: ${safe(typeof v==='object'?JSON.stringify(v):v)}</span>`).join('')||'<span class="muted">No payload</span>'}</div></td></tr>`).join(''):'<tr><td colspan="4" class="empty">No events in this view.</td></tr>';
}

$('#startFixture').addEventListener('click',async()=>{const button=$('#startFixture');button.disabled=true;try{const run=await api('/api/v1/fixtures',{method:'POST'});await loadRuns(run.id);announce(`Deterministic ${state.mode} fixture loaded from genuine API events.`)}catch(error){announce(error.message,true)}finally{button.disabled=false}});
$('#refresh').addEventListener('click',()=>loadRuns().catch(e=>announce(e.message,true)));
document.querySelectorAll('.tab').forEach(tab=>tab.addEventListener('click',async()=>{state.view=tab.dataset.view;document.querySelectorAll('.tab').forEach(x=>{x.classList.toggle('active',x===tab);x.setAttribute('aria-pressed',String(x===tab))});$('#replayControls').hidden=state.view!=='replay';$('#viewState').textContent=state.view==='replay'?'REPLAY':'LIVE';if(state.view==='replay'&&state.run){const upto=Number($('#replayRange').value);state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection?upto=${upto}`)}else if(state.run){state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection`)}render()}));
$('#replayRange').addEventListener('input',async e=>{$('#replayValue').value=e.target.value;if(state.run){state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection?upto=${e.target.value}`);render()}});
boot();
