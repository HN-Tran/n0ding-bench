const $ = (selector) => document.querySelector(selector);
const state = { runs: [], run: null, events: [], projection: null, view: 'latest', stream: null };

// All dynamic values pass through a text node before entering template markup.
function safe(value) { const node=document.createElement('span'); node.textContent=String(value??'—'); return node.innerHTML; }
function shortTime(value) { return value?new Intl.DateTimeFormat(undefined,{hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value)):'—'; }
function shortDate(value) { return value?new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value)):'—'; }
async function api(path,options) { const response=await fetch(path,options); const body=await response.json().catch(()=>({})); if(!response.ok) throw new Error(body.error||`${response.status} ${response.statusText}`); return body; }
function announce(message,error=false) { $('#notice').textContent=message; $('#notice').classList.toggle('error',error); }

async function boot() {
  try {
    const health=await api('/healthz');
    if(health.mode&&health.mode!=='bench') throw new Error('This interface requires Bench mode');
    $('#connectionDot').className='signal ok'; $('#connectionText').textContent='Connected';
    await loadRuns(); announce('Bench is connected. Start or select a run.');
  } catch(error) { $('#connectionDot').className='signal bad'; $('#connectionText').textContent='Offline'; announce(`Cannot reach backend: ${error.message}`,true); }
}

async function loadRuns(selectId) {
  const payload=await api('/api/v1/runs'); state.runs=payload.runs||[];
  $('#runs').innerHTML=state.runs.length?state.runs.slice().reverse().map(run=>`<button class="run-button ${state.run?.id===run.id?'active':''}" data-id="${safe(run.id)}"><span class="run-name">${safe(run.name||run.id)}</span><span class="run-meta"><span>BENCH</span><time>${safe(shortDate(run.created_at))}</time></span></button>`).join(''):'<p class="empty">No runs yet.</p>';
  document.querySelectorAll('.run-button').forEach(button=>button.addEventListener('click',()=>selectRun(button.dataset.id)));
  const wanted=selectId||state.run?.id; if(wanted) await selectRun(wanted); else if(state.runs.length) await selectRun(state.runs.at(-1).id);
}

async function selectRun(id) {
	if(state.stream) { state.stream.close(); state.stream=null; }
  state.run=state.runs.find(run=>run.id===id)||{id};
  const [eventsPayload,projection]=await Promise.all([api(`/api/v1/runs/${encodeURIComponent(id)}/events`),api(`/api/v1/runs/${encodeURIComponent(id)}/projection`)]);
  state.events=eventsPayload.events||[]; state.projection=projection;
  document.querySelectorAll('.run-button').forEach(button=>button.classList.toggle('active',button.dataset.id===id));
  const last=state.events.at(-1)?.sequence||0; $('#replayRange').max=last; $('#replayRange').value=last; $('#replayValue').value=last;
  $('#exportRun').href=`/api/v1/runs/${encodeURIComponent(id)}/export`; $('#exportRun').removeAttribute('aria-disabled');
  render(); announce(`Showing ${state.events.length} stored events for ${state.run.name||id}.`);
	openLiveStream(id);
}

function openLiveStream(id) {
  const after=state.events.at(-1)?.event_id||state.events.at(-1)?.sequence||0;
  const stream=new EventSource(`/api/v1/runs/${encodeURIComponent(id)}/events?after=${encodeURIComponent(after)}`);
  state.stream=stream;
  stream.onmessage=onStreamEvent;
  ['benchmark.started','benchmark.completed','benchmark.completed_with_errors','benchmark.cancelled','benchmark.interrupted','case.attempt','case.completed','case.failed','case.timed_out','case.cancelled','score.recorded','target.manifest','target.failed'].forEach(type=>stream.addEventListener(type,onStreamEvent));
  stream.onerror=()=>{ if(state.stream===stream) { $('#connectionDot').className='signal bad'; $('#connectionText').textContent='Reconnecting'; } };
  stream.onopen=()=>{ $('#connectionDot').className='signal ok'; $('#connectionText').textContent='Live'; };
}

async function onStreamEvent(message) {
  if(!state.run) return;
  try {
    const event=JSON.parse(message.data);
    if(!state.events.some(item=>item.event_id===event.event_id)) state.events.push(event);
    state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection`);
    const last=state.events.at(-1)?.sequence||0; $('#replayRange').max=last; if(state.view==='latest') { $('#replayRange').value=last; $('#replayValue').value=last; }
    render();
  } catch(error) { announce(`Live update failed: ${error.message}`,true); }
}

function visibleEvents() { if(state.view==='latest') return state.events; const upto=Number($('#replayRange').value); return state.events.filter(event=>event.sequence<=upto); }
function render() {
  const projection=state.projection||{}; const status=projection.status||'created';
  $('#status').textContent=status; $('#status').className=`status-${status}`; $('#steps').textContent=projection.steps||0; $('#lastEvent').textContent=projection.last_event_id||'—'; $('#created').textContent=shortDate(projection.run?.created_at||state.run?.created_at); $('#runId').textContent=state.run?.id||'No run'; $('#evidenceHeading').textContent=state.run?.name||'Select a run';
  renderBench(visibleEvents()); renderTimeline();
}

function renderBench(events) {
  if(!state.run) { $('#domainEvidence').innerHTML='<p class="empty">Choose a run to inspect its evidence.</p>'; return; }
  const started=events.find(event=>event.type==='benchmark.started')?.payload||{};
  const attempts=events.filter(event=>event.type.startsWith('case.')).map(event=>({source:event.sequence,status:event.type.slice(5),...event.payload}));
  const results=events.filter(event=>event.type==='score.recorded').map(event=>({source:event.sequence,...event.payload}));
  const failures=attempts.filter(attempt=>['failed','timed_out','cancelled'].includes(attempt.status));
  const targets=[...new Set(results.map(result=>result.target||result.model).filter(Boolean))];
  const scorers=[...new Set(results.map(result=>result.scorer_version).filter(Boolean))];
  const baseline=targets[0];
  const stats=targets.map(target=>{const rows=results.filter(result=>(result.target||result.model)===target);const scores=rows.map(row=>Number(row.score)).filter(Number.isFinite);return {target,samples:scores.length,score:scores.length?scores.reduce((a,b)=>a+b,0)/scores.length:NaN,failures:failures.filter(row=>(row.target||row.model)===target).length};});
  const baselineScore=stats.find(item=>item.target===baseline)?.score;
  const expected=Number(started.cases)||new Set(attempts.map(attempt=>attempt.case)).size;
  const warnings=[];
  if(scorers.length>1) warnings.push('Multiple scorer versions are visible; aggregate deltas may not be directly comparable.');
  if(stats.some(item=>expected&&item.samples<expected)) warnings.push('Sample counts differ or are incomplete; missing and failed samples are excluded from score averages.');
  if(!targets.length) warnings.push('No completed target samples are visible at this replay position.');
  const progress=expected&&targets.length?Math.min(100,Math.round(results.length/(expected*targets.length)*100)):(events.some(event=>event.type==='benchmark.completed')?100:0);
  const targetLabel=targets.join(', ')||(Array.isArray(started.targets)?started.targets.join(', '):started.targets)||started.models||'Not recorded';
  const catalog=`<div class="catalog-grid" aria-label="Evaluation configuration"><div><span>Dataset</span><strong>${safe(started.dataset||'Embedded fixture')}</strong></div><div><span>Suite</span><strong>${safe(started.suite||'Not recorded')}</strong></div><div><span>Targets</span><strong>${safe(targetLabel)}</strong></div><div><span>Scorers</span><strong>${safe(scorers.join(', ')||'Not observed')}</strong></div></div>`;
  const comparisonRows=stats.length?stats.map(item=>`<tr><td>${safe(item.target)}${item.target===baseline?' <span class="baseline">BASELINE</span>':''}</td><td>${item.samples}</td><td>${item.failures}</td><td>${Number.isFinite(item.score)?item.score.toFixed(3):'—'}</td><td class="delta">${item.target===baseline?'—':Number.isFinite(item.score)&&Number.isFinite(baselineScore)?`${item.score-baselineScore>=0?'+':''}${(item.score-baselineScore).toFixed(3)}`:'—'}</td></tr>`).join(''):'<tr><td colspan="5" class="empty">No completed samples to compare.</td></tr>';
  const rows=attempts.length?attempts.map(attempt=>{const observations=results.filter(score=>score.case===attempt.case&&score.target===(attempt.target||attempt.model));return `<tr><td>${safe(attempt.case)}</td><td><span class="case-status status-${safe(attempt.status)}">${safe(attempt.status)}</span></td><td>${safe(attempt.target||attempt.model)}</td><td>${safe(attempt.attempt||attempt.attempts)}</td><td>${safe(attempt.retry_of)}</td><td>${safe(observations.map(score=>score.scorer_version).join(', '))}</td><td>${safe(observations.map(score=>score.score).join(', '))}</td><td>${safe(observations.map(score=>score.evidence).filter(Boolean).join('; ')||attempt.error||(attempt.timeout_ms?`${attempt.timeout_ms} ms`:'—'))}</td><td class="mono">#${attempt.source}</td></tr>`;}).join(''):'<tr><td colspan="9" class="empty">No case attempts in this view.</td></tr>';
  $('#domainEvidence').innerHTML=`${catalog}<div class="progress-block"><div><span>Run progress</span><strong>${progress}%</strong></div><progress max="100" value="${progress}">${progress}%</progress><small>${results.length} recorded scores · ${failures.length} failed, timed out, or cancelled attempts</small></div>${warnings.length?`<div class="warnings" role="note"><strong>Comparison notes</strong><ul>${warnings.map(warning=>`<li>${safe(warning)}</li>`).join('')}</ul></div>`:''}<p class="section-label">Comparison · first observed target is the explicit baseline</p><div class="table-wrap"><table><thead><tr><th>Target</th><th>Samples included</th><th>Failed attempts</th><th>Mean score</th><th>Delta vs baseline</th></tr></thead><tbody>${comparisonRows}</tbody></table></div><p class="method-note">Mean scores include only visible <code>score.recorded</code> evidence. Failed, timed-out, cancelled, and missing samples remain visible below and are not silently scored as zero.</p><p class="section-label">All attempts and scorer provenance</p><div class="table-wrap"><table><thead><tr><th>Case</th><th>Status</th><th>Target</th><th>Attempt</th><th>Retry of</th><th>Scorer / version</th><th>Score</th><th>Evidence</th><th>Event</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function renderTimeline() {
  const events=visibleEvents();
  $('#timeline').innerHTML=events.length?events.map(event=>`<tr><td class="mono">${event.sequence}</td><td><time datetime="${safe(event.occurred_at)}">${safe(shortTime(event.occurred_at))}</time></td><td class="event-type">${safe(event.type)}</td><td><div class="payload">${Object.entries(event.payload||{}).map(([key,value])=>`<span class="datum" title="${safe(JSON.stringify(value))}">${safe(key)}: ${safe(typeof value==='object'?JSON.stringify(value):value)}</span>`).join('')||'<span class="muted">No payload</span>'}</div></td></tr>`).join(''):'<tr><td colspan="4" class="empty">No events in this view.</td></tr>';
}

$('#startFixture').addEventListener('click',async()=>{const button=$('#startFixture');button.disabled=true;try{const run=await api('/api/v1/fixtures',{method:'POST'});await loadRuns(run.id);announce('Deterministic Bench fixture loaded from genuine API events.');}catch(error){announce(error.message,true);}finally{button.disabled=false;}});
$('#refresh').addEventListener('click',()=>loadRuns().catch(error=>announce(error.message,true)));
document.querySelectorAll('.tab').forEach(tab=>tab.addEventListener('click',async()=>{state.view=tab.dataset.view;document.querySelectorAll('.tab').forEach(item=>{item.classList.toggle('active',item===tab);item.setAttribute('aria-pressed',String(item===tab));});$('#replayControls').hidden=state.view!=='replay';$('#viewState').textContent=state.view==='replay'?'REPLAY':'LIVE';if(state.run)state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection${state.view==='replay'?`?upto=${Number($('#replayRange').value)}`:''}`);render();}));
$('#replayRange').addEventListener('input',async event=>{$('#replayValue').value=event.target.value;if(state.run){state.projection=await api(`/api/v1/runs/${encodeURIComponent(state.run.id)}/projection?upto=${event.target.value}`);render();}});
boot();
