import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, isLoggedIn } from '../lib/api'

/* ── Scroll-triggered visibility hook ── */
function useReveal(threshold = 0.15) {
  const ref = useRef(null)
  const [visible, setVisible] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const obs = new IntersectionObserver(([e]) => { if (e.isIntersecting) { setVisible(true); obs.disconnect() } }, { threshold })
    obs.observe(el)
    return () => obs.disconnect()
  }, [threshold])
  return [ref, visible]
}

/* ── NLP examples ── */
const nlpExamples = [
  { q: 'Do we have any OS notes?', tags: [{ t: 'WHAT', v: 'OS notes', c: '#7F77DD' }, { t: 'WHY', v: 'check', c: '#BA7517' }] },
  { q: 'What did Priya upload yesterday?', tags: [{ t: 'WHO', v: 'Priya', c: '#D85A30' }, { t: 'WHEN', v: 'yesterday', c: '#1D9E75' }, { t: 'WHY', v: 'retrieve', c: '#BA7517' }] },
  { q: 'Find the compiler lab manual', tags: [{ t: 'WHAT', v: 'compiler lab', c: '#7F77DD' }, { t: 'WHY', v: 'search', c: '#BA7517' }] },
  { q: "What's new since Monday?", tags: [{ t: 'WHEN', v: 'since Monday', c: '#1D9E75' }, { t: 'WHY', v: 'activity', c: '#BA7517' }] },
]

/* ── Pipeline stages ── */
const pipeStages = [
  { t: 'Capture', d: 'Auto-track or pin', icon: 'fa-download', c: '#534AB7' },
  { t: 'Extract', d: 'Read inside PDF/DOCX', icon: 'fa-file-alt', c: '#534AB7' },
  { t: 'Classify', d: 'Claude AI + folders', icon: 'fa-brain', c: '#534AB7' },
  { t: 'Stage', d: '24h auto-commit', icon: 'fa-clock', c: '#0F6E56' },
  { t: 'Sync', d: 'Push to Drive', icon: 'fa-cloud-upload-alt', c: '#0F6E56' },
]

/* ── Notes Q&A examples ── */
const qaExamples = [
  { q: "Summarize Chapter 5", a: "Chapter 5 covers process scheduling algorithms: FCFS, SJF, Round Robin, and Priority Scheduling. Key concept: no single algorithm is optimal for all scenarios..." },
  { q: "What did the teacher say about integrals?", a: "From the uploaded lecture notes: Integration by parts is emphasized for the exam. The formula is uv - integral(v du). Practice problems focus on trigonometric substitution..." },
  { q: "Explain photosynthesis from our bio notes", a: "From Biology_Unit3.pdf: Photosynthesis occurs in two stages — light reactions (thylakoid) and Calvin cycle (stroma). Key equation: 6CO2 + 6H2O -> C6H12O6 + 6O2..." },
]

export default function Landing() {
  const [contact, setContact] = useState('')
  const [joined, setJoined] = useState(false)
  const navigate = useNavigate()

  const [heroRef, heroVis] = useReveal(0.1)
  const [probRef, probVis] = useReveal()
  const [nlpRef, nlpVis] = useReveal(0.1)
  const [pipeRef, pipeVis] = useReveal(0.1)
  const [capRef, capVis] = useReveal()
  const [qaRef, qaVis] = useReveal()
  const [deepRef, deepVis] = useReveal(0.05)
  const [techRef, techVis] = useReveal()
  const [ctaRef, ctaVis] = useReveal()

  const joinWaitlist = async () => {
    if (!contact.trim()) return
    try { await api.joinWaitlist(contact.trim()) } catch {}
    setJoined(true)
  }

  const rv = (show) => `rl-reveal ${show ? 'rl-vis' : ''}`
  const stg = (show) => show ? 'rl-stagger' : ''

  return (
    <div className="rl">
      <style>{`
        .rl{font-family:'Inter',-apple-system,sans-serif;color:#1a1a1a;background:#fff;overflow-x:hidden;position:relative}
        .rl a{text-decoration:none;color:inherit}

        /* ── Floating blobs ── */
        .rl-blobs{position:fixed;top:0;left:0;width:100%;height:100%;pointer-events:none;z-index:0;overflow:hidden}
        .rl-blob{position:absolute;border-radius:50%;filter:blur(100px);opacity:0.07;will-change:transform}
        .rl-blob--1{width:500px;height:500px;background:#25D366;top:-10%;right:5%;animation:blobDrift1 20s ease-in-out infinite}
        .rl-blob--2{width:400px;height:400px;background:#2563eb;bottom:10%;left:-5%;animation:blobDrift2 25s ease-in-out infinite}
        .rl-blob--3{width:350px;height:350px;background:#7F77DD;top:30%;right:-8%;animation:blobDrift3 18s ease-in-out infinite}
        .rl-blob--4{width:300px;height:300px;background:#D85A30;top:60%;left:10%;animation:blobDrift4 22s ease-in-out infinite}
        .rl-blob--5{width:250px;height:250px;background:#25D366;bottom:-5%;right:30%;animation:blobDrift5 16s ease-in-out infinite}
        @keyframes blobDrift1{0%,100%{transform:translate(0,0) scale(1)}25%{transform:translate(-60px,40px) scale(1.1)}50%{transform:translate(30px,80px) scale(0.95)}75%{transform:translate(50px,-30px) scale(1.05)}}
        @keyframes blobDrift2{0%,100%{transform:translate(0,0) scale(1)}33%{transform:translate(70px,-50px) scale(1.08)}66%{transform:translate(-40px,60px) scale(0.92)}}
        @keyframes blobDrift3{0%,100%{transform:translate(0,0)}25%{transform:translate(-50px,70px)}50%{transform:translate(40px,20px)}75%{transform:translate(-20px,-40px)}}
        @keyframes blobDrift4{0%,100%{transform:translate(0,0) scale(1)}50%{transform:translate(60px,-80px) scale(1.1)}}
        @keyframes blobDrift5{0%,100%{transform:translate(0,0)}33%{transform:translate(-30px,-60px)}66%{transform:translate(50px,30px)}}

        /* ── Reveal animations ── */
        .rl-reveal{opacity:0;transform:translateY(32px);transition:opacity .7s cubic-bezier(.16,1,.3,1),transform .7s cubic-bezier(.16,1,.3,1)}
        .rl-vis{opacity:1;transform:translateY(0)}
        .rl-stagger>*{opacity:0;transform:translateY(20px);animation:rlUp .6s cubic-bezier(.16,1,.3,1) forwards}
        @keyframes rlUp{to{opacity:1;transform:translateY(0)}}
        .rl-dim{opacity:0;transform:translateY(24px) scale(.95);animation:rlDim .5s cubic-bezier(.16,1,.3,1) forwards}
        @keyframes rlDim{to{opacity:1;transform:translateY(0) scale(1)}}
        .rl-pipe{opacity:0;transform:translateX(-20px);animation:rlPipe .5s cubic-bezier(.16,1,.3,1) forwards}
        @keyframes rlPipe{to{opacity:1;transform:translateX(0)}}
        @keyframes rlPulse{0%,100%{background:rgba(37,211,102,.08)}50%{background:rgba(37,211,102,.22)}}
        .rl-match{animation:rlPulse 2s ease infinite;border-radius:4px;padding:2px 8px}
        .rl-tag{display:inline-block;font-size:11px;font-weight:600;padding:4px 12px;border-radius:20px}

        /* ── Layout ── */
        .rl-s{max-width:860px;margin:0 auto;padding:80px 28px;position:relative;z-index:1}

        /* ── Hero glow ring ── */
        .rl-hero-glow{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);width:600px;height:600px;background:radial-gradient(circle,rgba(37,211,102,0.06) 0%,transparent 70%);pointer-events:none;z-index:0}

        /* ── Hero badge shimmer ── */
        .rl-badge{display:inline-flex;align-items:center;gap:8px;font-size:12px;font-weight:600;color:#25D366;background:linear-gradient(135deg,#f0fdf4 0%,#dcfce7 100%);border:1px solid #bbf7d0;padding:6px 20px;border-radius:50px;position:relative;overflow:hidden}
        .rl-badge::after{content:'';position:absolute;top:0;left:-100%;width:60%;height:100%;background:linear-gradient(90deg,transparent,rgba(255,255,255,0.6),transparent);animation:shimmer 3s ease-in-out infinite}
        @keyframes shimmer{0%{left:-100%}50%,100%{left:150%}}

        /* ── Q&A chat bubbles ── */
        .rl-qa-q{background:#111;color:#fff;padding:12px 20px;border-radius:18px 18px 4px 18px;font-size:14px;font-weight:500;max-width:320px;margin-left:auto;margin-bottom:10px}
        .rl-qa-a{background:#f0fdf4;border:1px solid #bbf7d0;padding:14px 20px;border-radius:18px 18px 18px 4px;font-size:13px;line-height:1.75;color:#333;max-width:440px;margin-bottom:8px}
        .rl-qa-a strong{color:#0F6E56}

        /* ── Typing animation ── */
        .rl-typing{display:inline-flex;gap:4px;padding:12px 20px}
        .rl-typing span{width:6px;height:6px;background:#25D366;border-radius:50%;animation:typingBounce 1.4s infinite}
        .rl-typing span:nth-child(2){animation-delay:.2s}
        .rl-typing span:nth-child(3){animation-delay:.4s}
        @keyframes typingBounce{0%,60%,100%{transform:translateY(0)}30%{transform:translateY(-8px)}}

        /* ── Section divider ── */
        .rl-divider{height:1px;background:linear-gradient(90deg,transparent,#ddd,transparent);margin:0}

        @media(max-width:640px){
          .rl-s{padding:48px 18px}
          .rl-blob--1,.rl-blob--3{display:none}
          .rl-dims,.rl-prow{flex-direction:column}
          .rl-parr{display:none}
          .rl-capg{grid-template-columns:1fr!important}
          .rl-techg{grid-template-columns:1fr 1fr!important}
          .rl-hero-glow{width:300px;height:300px}
          .rl-qa-q{max-width:260px}
          .rl-qa-a{max-width:100%}
        }
      `}</style>

      {/* ── Floating background blobs ── */}
      <div className="rl-blobs">
        <div className="rl-blob rl-blob--1"/>
        <div className="rl-blob rl-blob--2"/>
        <div className="rl-blob rl-blob--3"/>
        <div className="rl-blob rl-blob--4"/>
        <div className="rl-blob rl-blob--5"/>
      </div>

      {/* Nav */}
      <nav style={{position:'sticky',top:0,zIndex:100,background:'rgba(255,255,255,0.85)',backdropFilter:'blur(20px)',borderBottom:'1px solid rgba(0,0,0,0.06)',padding:'0 32px'}}>
        <div style={{maxWidth:1100,margin:'0 auto',display:'flex',alignItems:'center',justifyContent:'space-between',height:56}}>
          <a href="#" style={{fontWeight:900,fontSize:20,letterSpacing:-.5,display:'flex',alignItems:'center',gap:6}}>
            <i className="fas fa-crown" style={{color:'#25D366',fontSize:16}}/> Reyna <span style={{fontSize:11,color:'#999',fontWeight:400}}>v2</span>
          </a>
          <div style={{display:'flex',gap:24,alignItems:'center'}}>
            {[
              {label:'Pipeline',href:'#pipeline'},
              {label:'How it works',href:'#deepdive'},
              {label:'Features',href:'#features'},
              {label:'Architecture',href:'#architecture'},
            ].map(t=><a key={t.label} href={t.href} style={{fontSize:13,color:'#666',fontWeight:500,transition:'color .2s'}}>{t.label}</a>)}
            {isLoggedIn()
              ? <button onClick={()=>navigate('/dashboard')} style={{fontSize:13,fontWeight:700,color:'#fff',background:'#111',padding:'7px 18px',borderRadius:6,border:'none',cursor:'pointer'}}>Dashboard <i className="fas fa-arrow-right" style={{fontSize:10,marginLeft:2}}/></button>
              : <button onClick={()=>navigate('/login')} style={{fontSize:13,fontWeight:700,color:'#fff',background:'#111',padding:'7px 18px',borderRadius:6,border:'none',cursor:'pointer'}}>Sign in</button>
            }
          </div>
        </div>
      </nav>

      {/* ═══ S1: HERO ═══ */}
      <div ref={heroRef} className={rv(heroVis)} style={{position:'relative',overflow:'hidden'}}>
        <div className="rl-hero-glow"/>
        <div className="rl-s" style={{padding:'120px 28px 80px',textAlign:'center'}}>
          <div className={stg(heroVis)}>
            <div className="rl-badge" style={{marginBottom:32}}>
              <i className="fas fa-robot" style={{fontSize:10}}/> Five autonomous agents. One pipeline.
            </div>
            <h1 style={{fontSize:'clamp(36px,5.5vw,60px)',fontWeight:900,lineHeight:1.05,letterSpacing:-2.5,marginBottom:28,animationDelay:'120ms'}}>
              Your WhatsApp group's files<br/>
              <span style={{background:'linear-gradient(135deg, #25D366, #0F6E56)',WebkitBackgroundClip:'text',WebkitTextFillColor:'transparent'}}>understood, organized, searchable.</span>
            </h1>
            <p style={{fontSize:18,lineHeight:1.8,color:'#555',maxWidth:640,margin:'0 auto 16px',animationDelay:'240ms'}}>
              Reyna packs the entire study-file workflow into one place. <strong style={{color:'#1a1a1a'}}>Stop hunting through chat history. Stop re-uploading to Drive. Stop typing answers from notes you can't find.</strong> Reyna does all of it — capture, parse, dedupe, classify, file, retrieve, and answer — autonomously.
            </p>
            <p style={{fontSize:15,color:'#999',maxWidth:540,margin:'0 auto 40px',animationDelay:'360ms',letterSpacing:0.3}}>
              Zero commands. Zero training. Zero new apps to learn.
            </p>
            <div style={{display:'flex',gap:14,justifyContent:'center',flexWrap:'wrap',animationDelay:'480ms'}}>
              <a href="#waitlist" style={{fontSize:15,fontWeight:700,color:'#fff',background:'#25D366',padding:'14px 36px',borderRadius:10,display:'inline-flex',alignItems:'center',gap:8,transition:'transform .2s,box-shadow .2s',textDecoration:'none'}}>Get early access <i className="fas fa-arrow-right" style={{fontSize:12}}/></a>
              <a href="#pipeline" style={{fontSize:15,fontWeight:600,color:'#1a1a1a',background:'#fff',padding:'14px 36px',borderRadius:10,border:'1px solid #ddd',display:'inline-flex',alignItems:'center',gap:8,transition:'transform .2s,border-color .2s',textDecoration:'none'}}><i className="fas fa-sitemap" style={{fontSize:12}}/> See the pipeline</a>
            </div>

            {/* Mini feature pills under hero */}
            <div style={{display:'flex',gap:10,justifyContent:'center',flexWrap:'wrap',marginTop:48,animationDelay:'600ms'}}>
              {[
                {icon:'fa-brain',t:'AI Classification'},
                {icon:'fa-comments',t:'NLP Retrieval'},
                {icon:'fa-file-alt',t:'Content Extraction'},
                {icon:'fa-question-circle',t:'Notes Q&A'},
                {icon:'fa-folder-open',t:'Smart Folders'},
              ].map((f,i)=>(
                <span key={i} style={{fontSize:12,fontWeight:500,color:'#666',background:'rgba(255,255,255,0.7)',backdropFilter:'blur(8px)',border:'1px solid #eee',padding:'6px 14px',borderRadius:20,display:'flex',alignItems:'center',gap:6}}>
                  <i className={`fas ${f.icon}`} style={{fontSize:10,color:'#25D366'}}/> {f.t}
                </span>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S2: PROBLEM ═══ */}
      <div ref={probRef} className={rv(probVis)}>
        <div className="rl-s">
          <div style={{fontSize:12,fontWeight:700,color:'#25D366',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-exclamation-triangle" style={{marginRight:6}}/>The problem</div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:24,letterSpacing:-1}}>Every group chat is a graveyard of lost files.</h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:16}}>WhatsApp is where 2 billion people share files. Study notes, project docs, client deliverables — they all go into group chats. And then they <strong style={{color:'#ff4444'}}>disappear</strong>. Buried under messages, impossible to search, auto-deleted after 30 days.</p>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555'}}>The same PDF gets shared five times because nobody can find it. <strong style={{color:'#ff4444'}}>Sound familiar?</strong></p>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S3: NLP RETRIEVAL ═══ */}
      <div id="pipeline" style={{background:'rgba(250,250,250,0.7)',backdropFilter:'blur(4px)'}}>
        <div ref={nlpRef} className={`rl-s ${rv(nlpVis)}`}>
          <div style={{fontSize:12,fontWeight:700,color:'#D85A30',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-comments" style={{marginRight:6}}/>Killer feature #1</div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:12,letterSpacing:-1}}>Ask for files like you'd ask a friend.</h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:36}}>Not keyword search. Reyna's NLP engine resolves <strong>who</strong> shared it, <strong>what</strong> it's about, <strong>when</strong> it was shared, and <strong>why</strong> you're asking — from a single natural sentence.</p>

          <div style={{textAlign:'center',marginBottom:24}}>
            <div style={{display:'inline-block',fontSize:17,fontWeight:500,padding:'14px 28px',borderRadius:28,background:'#fff',border:'1px solid #ddd',boxShadow:'0 2px 12px rgba(0,0,0,0.04)'}}>
              <i className="fas fa-quote-left" style={{fontSize:10,color:'#ccc',marginRight:8}}/>
              What did <span style={{color:'#D85A30',fontWeight:700}}>Rahul</span> share about <span style={{color:'#7F77DD',fontWeight:700}}>drones</span> <span style={{color:'#1D9E75',fontWeight:700}}>last week</span>?
              <i className="fas fa-quote-right" style={{fontSize:10,color:'#ccc',marginLeft:8}}/>
            </div>
          </div>

          <div className="rl-dims" style={{display:'flex',gap:12,justifyContent:'center',marginBottom:28}}>
            {[
              {t:'WHO',v:'Rahul',d:'sender filter',c:'#D85A30',bg:'#FAECE7',icon:'fa-user'},
              {t:'WHAT',v:'drones',d:'content + filename',c:'#7F77DD',bg:'#EEEDFE',icon:'fa-file-alt'},
              {t:'WHEN',v:'last week',d:'7-day window',c:'#1D9E75',bg:'#E1F5EE',icon:'fa-calendar'},
              {t:'WHY',v:'retrieve',d:'intent: find file',c:'#BA7517',bg:'#FAEEDA',icon:'fa-bullseye'},
            ].map((dim,i)=>(
              <div key={dim.t} className={nlpVis?'rl-dim':''} style={{flex:1,minWidth:120,padding:'18px 14px',textAlign:'center',borderTop:`3px solid ${dim.c}`,background:'#fff',borderRadius:'0 0 10px 10px',boxShadow:'0 2px 8px rgba(0,0,0,0.03)',animationDelay:`${300+i*150}ms`}}>
                <i className={`fas ${dim.icon}`} style={{fontSize:16,color:dim.c,marginBottom:8,display:'block'}}/>
                <div style={{fontSize:11,fontWeight:700,letterSpacing:2,textTransform:'uppercase',color:dim.c,marginBottom:4}}>{dim.t}</div>
                <div style={{fontSize:18,fontWeight:700,color:'#1a1a1a'}}>{dim.v}</div>
                <div style={{fontSize:11,color:'#999',marginTop:4}}>{dim.d}</div>
              </div>
            ))}
          </div>

          <div style={{background:'#fff',border:'1px solid #e0ddd5',borderRadius:12,padding:'16px 24px',maxWidth:560,margin:'0 auto 36px',fontSize:14,color:'#555',lineHeight:1.8,boxShadow:'0 2px 12px rgba(0,0,0,0.03)'}}>
            <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:8}}><i className="fas fa-crown" style={{color:'#25D366',fontSize:12}}/><strong style={{color:'#1a1a1a'}}>Reyna responds:</strong></div>
            Found <strong>2 files</strong> from <strong>Rahul</strong> matching <strong>"drones"</strong> in the last 7 days:<br/>
            <code style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,background:'#f5f5f5',padding:'2px 6px',borderRadius:4}}>Drone_Regulation_India_2024.pdf</code> — Robotics folder<br/>
            <code style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,background:'#f5f5f5',padding:'2px 6px',borderRadius:4}}>UAV_Project_Proposal_v2.docx</code> — Projects folder
          </div>

          <div style={{fontSize:11,fontWeight:700,color:'#888',letterSpacing:2,textTransform:'uppercase',textAlign:'center',marginBottom:16}}>More examples — every query is a conversation</div>
          <div style={{maxWidth:600,margin:'0 auto'}}>
            {nlpExamples.map((ex,i)=>(
              <div key={i} className={nlpVis?'rl-dim':''} style={{display:'flex',alignItems:'center',gap:14,marginBottom:8,padding:'10px 16px',background:'#fff',borderRadius:10,border:'1px solid #eee',animationDelay:`${800+i*120}ms`}}>
                <div style={{flex:2,fontSize:13,fontWeight:500}}>{ex.q}</div>
                <div style={{color:'#ccc',fontSize:14}}><i className="fas fa-arrow-right"/></div>
                <div style={{flex:3,display:'flex',gap:4,flexWrap:'wrap'}}>
                  {ex.tags.map((tag,j)=>(<span key={j} className="rl-tag" style={{background:tag.c+'15',color:tag.c}}>{tag.t}: {tag.v}</span>))}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ═══ S4: CLASSIFICATION PIPELINE ═══ */}
      <div id="features">
        <div ref={pipeRef} className={`rl-s ${rv(pipeVis)}`}>
          <div style={{fontSize:12,fontWeight:700,color:'#534AB7',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-brain" style={{marginRight:6}}/>Killer feature #2 — Autonomous pipeline</div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:12,letterSpacing:-1}}>Drop a file. Five agents handle everything.</h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:36}}>No commands. No folder selection. Reyna reads the file, understands what it's about, finds the best folder in your Drive, and syncs it — autonomously.</p>

          <div className="rl-prow" style={{display:'flex',gap:0,marginBottom:32}}>
            {pipeStages.map((s,i)=>(
              <div key={s.t} style={{display:'contents'}}>
                <div className={pipeVis?'rl-pipe':''} style={{flex:1,textAlign:'center',padding:'20px 10px',background:i<3?'#EEEDFE':'#E1F5EE',borderRadius:i===0?'12px 0 0 12px':i===4?'0 12px 12px 0':0,animationDelay:`${200+i*180}ms`}}>
                  <i className={`fas ${s.icon}`} style={{fontSize:20,color:s.c,marginBottom:8,display:'block'}}/>
                  <div style={{fontSize:15,fontWeight:700,color:s.c}}>{s.t}</div>
                  <div style={{fontSize:11,color:i<3?'#7F77DD':'#1D9E75',marginTop:4}}>{s.d}</div>
                </div>
                {i<4&&<div className="rl-parr" style={{display:'flex',alignItems:'center',background:i<2?'#EEEDFE':i===2?'#dde8dd':'#E1F5EE'}}><i className="fas fa-chevron-right" style={{fontSize:10,color:'#aaa'}}/></div>}
              </div>
            ))}
          </div>

          {/* Content extraction + classification demo */}
          <div style={{background:'#fafafa',border:'1px solid #eee',borderRadius:14,padding:'24px 28px',marginBottom:24}}>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:14,fontWeight:600,marginBottom:14,display:'flex',alignItems:'center',gap:8}}>
              <i className="fas fa-file-pdf" style={{color:'#dc2626'}}/> Module3_Compiler_Design_Unit2.pdf
            </div>
            {[
              {icon:'fa-download',c:'#534AB7',bg:'#EEEDFE',t:'File shared in "CSE 2026 — Section B". <strong>Auto-track on</strong> — captured.'},
              {icon:'fa-file-alt',c:'#534AB7',bg:'#EEEDFE',t:'<strong>Content extracted</strong>: 42 pages — <strong>syntax analysis, parsing tables, LL(1) grammars</strong>.'},
              {icon:'fa-brain',c:'#534AB7',bg:'#EEEDFE',t:'Claude receives extracted content + filename + user\'s existing folders:'},
            ].map((step,i)=>(
              <div key={i} style={{display:'flex',alignItems:'flex-start',gap:10,marginBottom:8}}>
                <div style={{width:26,height:26,borderRadius:'50%',background:step.bg,display:'flex',alignItems:'center',justifyContent:'center',flexShrink:0}}><i className={`fas ${step.icon}`} style={{fontSize:10,color:step.c}}/></div>
                <div style={{fontSize:13,lineHeight:1.7,color:'#555'}} dangerouslySetInnerHTML={{__html:step.t}}/>
              </div>
            ))}
            <div style={{background:'#fff',border:'1px solid #ddd',borderRadius:8,padding:'12px 18px',margin:'8px 0 12px 36px',fontFamily:'JetBrains Mono,monospace',fontSize:12,lineHeight:2.2,color:'#888'}}>
              <i className="fas fa-folder" style={{color:'#25D366',marginRight:4}}/> Reyna/<br/>
              &ensp;&ensp;<i className="fas fa-folder" style={{color:'#888',marginRight:4}}/> DSA/<br/>
              &ensp;&ensp;<span className="rl-match"><i className="fas fa-folder-open" style={{color:'#25D366',marginRight:4}}/> <strong style={{color:'#085041'}}>Compiler Design/</strong></span> <span style={{color:'#25D366',fontSize:11}}>← best match</span><br/>
              &ensp;&ensp;<i className="fas fa-folder" style={{color:'#888',marginRight:4}}/> Operating Systems/<br/>
              &ensp;&ensp;<i className="fas fa-folder" style={{color:'#888',marginRight:4}}/> DBMS/
            </div>
            {[
              {icon:'fa-clock',c:'#0F6E56',bg:'#E1F5EE',t:'<strong>Staged</strong> as Compiler Design / Module3_...pdf. Auto-commit: 24h.'},
              {icon:'fa-cloud-upload-alt',c:'#0F6E56',bg:'#E1F5EE',t:'<strong>Synced</strong> to Google Drive. Re-uploads become v2 automatically.'},
            ].map((step,i)=>(
              <div key={i} style={{display:'flex',alignItems:'flex-start',gap:10,marginBottom:6}}>
                <div style={{width:26,height:26,borderRadius:'50%',background:step.bg,display:'flex',alignItems:'center',justifyContent:'center',flexShrink:0}}><i className={`fas ${step.icon}`} style={{fontSize:10,color:step.c}}/></div>
                <div style={{fontSize:13,lineHeight:1.7,color:'#555'}} dangerouslySetInnerHTML={{__html:step.t}}/>
              </div>
            ))}
          </div>

          {/* Folder priority logic */}
          <div style={{fontSize:11,fontWeight:700,color:'#888',letterSpacing:2,textTransform:'uppercase',textAlign:'center',marginBottom:14}}>Folder priority logic</div>
          <div style={{display:'flex',gap:10,maxWidth:560,margin:'0 auto'}}>
            {[
              {n:'1st',l:'User-created folders',d:'Your structure wins',c:'#0F6E56',icon:'fa-user'},
              {n:'2nd',l:'Reyna-created folders',d:'From past classifications',c:'#534AB7',icon:'fa-crown'},
              {n:'3rd',l:'Create new folder',d:'Only when nothing fits',c:'#BA7517',icon:'fa-plus'},
            ].map((p,i)=>(
              <div key={i} style={{flex:1,padding:'16px 12px',textAlign:'center',borderTop:`3px solid ${p.c}`,background:'#fafafa',borderRadius:'0 0 10px 10px'}}>
                <i className={`fas ${p.icon}`} style={{fontSize:14,color:p.c,marginBottom:6,display:'block'}}/>
                <div style={{fontSize:22,fontWeight:800,color:p.c}}>{p.n}</div>
                <div style={{fontSize:12,fontWeight:600,marginTop:4}}>{p.l}</div>
                <div style={{fontSize:10,color:'#aaa',marginTop:2}}>{p.d}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S5: CAPTURE MODES ═══ */}
      <div style={{background:'rgba(250,250,250,0.7)',backdropFilter:'blur(4px)'}}>
        <div ref={capRef} className={`rl-s ${rv(capVis)}`}>
          <div style={{fontSize:12,fontWeight:700,color:'#25D366',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-bolt" style={{marginRight:6}}/>Two capture modes</div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:12,letterSpacing:-1}}>Choose your level of automation.</h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:36}}>Toggle per group from the dashboard. Both feed into the same autonomous pipeline.</p>
          <div className="rl-capg" style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:20}}>
            {[
              {num:'01',icon:'fa-folder',title:'Track all files',sub:'Full autopilot',color:'#25D366',desc:'Every document shared in the group is automatically captured, classified, and synced. Zero effort.',tip:'Best for active study groups'},
              {num:'02',icon:'fa-thumbtack',title:'Reactions only',sub:'Selective control',color:'#0ea5e9',desc:'Only files that get a pin reaction are staged. Everything else is ignored. You choose what matters.',tip:'Best for noisy or mixed groups'},
            ].map((m,i)=>(
              <div key={i} style={{background:'#fff',border:'2px solid #111',borderRadius:14,padding:28,position:'relative',overflow:'hidden'}}>
                <div style={{position:'absolute',top:12,right:16,fontSize:56,fontWeight:900,color:'#f5f5f5',lineHeight:1}}>{m.num}</div>
                <i className={`fas ${m.icon}`} style={{fontSize:28,color:'#111',marginBottom:12,display:'block'}}/>
                <h3 style={{fontSize:20,fontWeight:800,marginBottom:4}}>{m.title}</h3>
                <div style={{fontSize:12,fontWeight:700,color:m.color,textTransform:'uppercase',letterSpacing:2,marginBottom:14}}>{m.sub}</div>
                <p style={{fontSize:14,color:'#555',lineHeight:1.8,marginBottom:16}}>{m.desc}</p>
                <div style={{fontSize:13,color:m.color,fontWeight:500}}><i className="fas fa-arrow-right" style={{marginRight:6}}/> {m.tip}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S6: NOTES Q&A (replaces Insights) ═══ */}
      <div ref={qaRef} className={`rl-s ${rv(qaVis)}`}>
        <div style={{fontSize:12,fontWeight:700,color:'#7F77DD',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-graduation-cap" style={{marginRight:6}}/>Killer feature #3 — Notes Q&A</div>
        <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:12,letterSpacing:-1}}>Ask anything from your notes.</h2>
        <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:36}}>
          Ask Reyna about shared notes directly in WhatsApp. She fetches the relevant files from Drive, sends the content to Claude, and replies with a clear answer — instantly. No need to open any other app or copy files elsewhere.
        </p>

        {/* Chat-style Q&A demo */}
        <div style={{maxWidth:520,margin:'0 auto 32px',display:'flex',flexDirection:'column',gap:4}}>
          {qaExamples.map((ex,i)=>(
            <div key={i} className={qaVis?'rl-dim':''} style={{animationDelay:`${300+i*350}ms`}}>
              <div className="rl-qa-q">
                <i className="fas fa-user" style={{fontSize:9,opacity:0.5,marginRight:6}}/>{ex.q}
              </div>
              <div style={{display:'flex',alignItems:'flex-start',gap:8,marginBottom:16}}>
                <div style={{width:24,height:24,borderRadius:'50%',background:'#25D366',display:'flex',alignItems:'center',justifyContent:'center',flexShrink:0,marginTop:2}}>
                  <i className="fas fa-crown" style={{fontSize:9,color:'#fff'}}/>
                </div>
                <div className="rl-qa-a">
                  <div style={{fontSize:11,fontWeight:700,color:'#25D366',marginBottom:6,letterSpacing:1,textTransform:'uppercase'}}>Reyna</div>
                  {ex.a}
                </div>
              </div>
            </div>
          ))}
        </div>

        <div style={{textAlign:'center',padding:'16px 24px',background:'#f5f3ff',border:'1px solid #e0dafe',borderRadius:12,maxWidth:480,margin:'0 auto'}}>
          <div style={{fontSize:13,color:'#534AB7',fontWeight:600,marginBottom:4}}>
            <i className="fas fa-magic" style={{marginRight:6}}/>Powered by content extraction
          </div>
          <div style={{fontSize:12,color:'#888',lineHeight:1.6}}>
            Reyna reads every page of every PDF and DOCX shared in your group. When you ask a question, it searches through all extracted content — not just filenames.
          </div>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S6.5: DEEP DIVE — THE REAL PIPELINE ═══ */}
      <div id="deepdive" style={{background:'rgba(250,250,250,0.7)',backdropFilter:'blur(4px)'}}>
        <div ref={deepRef} className={`rl-s ${rv(deepVis)}`} style={{maxWidth:920}}>
          <div style={{fontSize:12,fontWeight:700,color:'#534AB7',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}>
            <i className="fas fa-cogs" style={{marginRight:6}}/>under the hood — what actually happens
          </div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,38px)',fontWeight:800,lineHeight:1.15,marginBottom:14,letterSpacing:-1}}>
            It's not "save the file and ping an LLM."<br/>
            <span style={{background:'linear-gradient(135deg,#7F77DD,#534AB7)',WebkitBackgroundClip:'text',WebkitTextFillColor:'transparent'}}>It's a 7-stage pipeline.</span>
          </h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#555',marginBottom:14}}>
            Every file that lands in Reyna goes through seven distinct processing stages before it hits your Drive. Six of them never touch an LLM at all. Here's the whole loop, step-by-step — explained for humans, not engineers.
          </p>
          <p style={{fontSize:14,lineHeight:1.7,color:'#888',marginBottom:36}}>
            <i className="fas fa-info-circle" style={{color:'#7F77DD',marginRight:6}}/>
            Each stage is a real piece of code in the open-source repo — not a marketing diagram.
          </p>

          {/* The 7 stages, vertically stacked */}
          <div style={{display:'flex',flexDirection:'column',gap:18}}>
            {[
              {
                num: '01',
                color: '#534AB7',
                bg: '#EEEDFE',
                icon: 'fa-fingerprint',
                title: 'Cryptographic deduplication',
                lay: 'If you (or anyone in your group) shares the exact same file twice, Reyna recognises it instantly — even if it has a different name — and refuses to create a duplicate.',
                tech: ['SHA-256 hash of every byte', 'Partial UNIQUE index in SQLite', 'Per-group sync.Mutex (race-condition proof)', 'Same name + different bytes → auto v2 / v3 versioning'],
                why: 'Stops Drive clutter at the source. Six identical "Module5.pdf" uploads in one minute? One row, one Drive file. Forever.',
              },
              {
                num: '02',
                color: '#7F77DD',
                bg: '#EEEDFE',
                icon: 'fa-file-zipper',
                title: 'Document parsing — without the API',
                lay: 'Office files (.docx, .pptx, .xlsx) are actually zip archives full of XML. Reyna unzips them, walks the inside, and pulls the real text out — entirely on its own server.',
                tech: ['archive/zip + encoding/xml from Go stdlib', 'Walks word/document.xml, ppt/slides/slide*.xml, xl/sharedStrings.xml', 'Strips XML markup with the streaming token decoder', 'Zero external libraries, zero API calls, zero cents'],
                why: 'PDFs go to Gemini for vision-aware content reading. Office docs get parsed locally in milliseconds for free. Best of both worlds.',
              },
              {
                num: '03',
                color: '#0F6E56',
                bg: '#E1F5EE',
                icon: 'fa-language',
                title: 'Tokenization & cleaning',
                lay: 'When you ask "give me the exact oscillator definition mohit sent today", Reyna throws away the noise words ("give", "exact", "definition", "today") and keeps only what actually identifies the file ("oscillator", "mohit", "today").',
                tech: ['80-word stopword filter (English + Hindi/Hinglish)', 'Drops question-style verbs, generic file vocab, time generics', 'Lowercase + punctuation strip + dedupe', 'Min-length 3 chars · falls back to raw phrase if everything filters'],
                why: 'Without this, the search would try to literally match "give exact oscillator definition" as a phrase and find nothing. With it, the one real keyword "oscillator" lights up the right file.',
              },
              {
                num: '04',
                color: '#D85A30',
                bg: '#FAECE7',
                icon: 'fa-magnifying-glass-chart',
                title: 'Ranked information retrieval',
                lay: 'Reyna scores every file in your library against your question — looking at filename, subject, tags, the document\'s extracted content, and its summary — then sorts the matches by how many of your keywords landed.',
                tech: ['OR-tokenized SQL with hand-built CASE expression scoring', 'ORDER BY (sum of CASE WHEN ... THEN 1 ELSE 0 END) DESC', 'LEFT JOIN against the users table for sender resolution', 'Time window + sender filter applied as separate predicates'],
                why: 'This is real information retrieval, not "send the question to an LLM and pray". The ranking happens in SQL — fast, deterministic, free.',
              },
              {
                num: '05',
                color: '#BA7517',
                bg: '#FAEEDA',
                icon: 'fa-folder-tree',
                title: 'Fuzzy folder normalization',
                lay: "When Gemini suggests a folder name like \"C Programming Lab\" and you already have \"C Programming Laboratory\", Reyna recognises they're the same thing and snaps the new file into the existing folder. No duplicate folders, ever.",
                tech: ['Token-based Jaccard similarity (intersection / union)', 'Strict subset detection — handles "Python Programming" ⊂ "Python Programming Modules"', 'Case-insensitive exact match short-circuit', 'Snap threshold ≥ 0.6 with explicit logging for audit'],
                why: 'Without this, two PDFs about the same subject would scatter into 5 near-identical folders. With it, your Drive stays clean automatically.',
              },
              {
                num: '06',
                color: '#534AB7',
                bg: '#EEEDFE',
                icon: 'fa-brain',
                title: 'Multi-source LLM call',
                lay: 'Only NOW does Gemini get involved. And it doesn\'t just see the file — it sees the file content, the filename, who sent it, when, in which group, plus the list of folders that already exist. Real context, real decision.',
                tech: ['Gemini 2.5 Flash with inline document blocks (PDFs)', 'Pre-extracted text payload for office docs', 'Sender + timestamp + group + existing-folders metadata in prompt', 'Strict JSON response mode with thinking-budget=0 for speed'],
                why: 'A naive system would ask "what subject is this PDF?" and get a generic answer. Reyna asks "given that Mohit usually shares Python files in the BPLC105B group, and the existing folders are X/Y/Z, where should this go?" — and gets a precise one.',
              },
              {
                num: '07',
                color: '#25D366',
                bg: '#E1F5EE',
                icon: 'fa-cloud-arrow-up',
                title: 'Drive sync + auto-public links',
                lay: "Once classified, the file is uploaded to your Google Drive in the right folder, and Reyna automatically makes it shareable by link — so when she drops a link in WhatsApp, it just works. No \"request access\" popups.",
                tech: ['Google Drive API v3 with OAuth 2.0', 'Multipart upload with name + parents metadata', 'Idempotent permissions.create with role=reader, type=anyone', 'Eventual-consistency-aware folder lookup (DB-first, Drive-second)'],
                why: 'Most "save to Drive" tools leave you fighting Drive\'s sharing settings. Reyna handles it once, automatically, on every upload.',
              },
            ].map((stage, i) => (
              <div key={stage.num} className={deepVis?'rl-dim':''} style={{
                background:'#fff',
                border:'1px solid #e6e4dc',
                borderLeft:`4px solid ${stage.color}`,
                borderRadius:12,
                padding:'24px 28px',
                animationDelay:`${100+i*120}ms`,
                boxShadow:'0 1px 8px rgba(0,0,0,0.03)',
              }}>
                <div style={{display:'flex',alignItems:'flex-start',gap:18}}>
                  {/* Stage number + icon */}
                  <div style={{flexShrink:0,width:56,textAlign:'center'}}>
                    <div style={{
                      width:48,height:48,borderRadius:12,
                      background:stage.bg,
                      display:'flex',alignItems:'center',justifyContent:'center',
                      marginBottom:6,
                    }}>
                      <i className={`fas ${stage.icon}`} style={{fontSize:18,color:stage.color}}/>
                    </div>
                    <div style={{fontSize:11,fontWeight:800,letterSpacing:1,color:stage.color}}>{stage.num}</div>
                  </div>
                  {/* Title + plain-language explanation */}
                  <div style={{flex:1,minWidth:0}}>
                    <h3 style={{fontSize:18,fontWeight:800,color:'#1a1a1a',marginBottom:8,letterSpacing:-0.3}}>{stage.title}</h3>
                    <p style={{fontSize:14,lineHeight:1.7,color:'#555',marginBottom:14}}>{stage.lay}</p>
                    {/* Tech bullets */}
                    <div style={{display:'flex',flexWrap:'wrap',gap:6,marginBottom:12}}>
                      {stage.tech.map((t,j)=>(
                        <span key={j} style={{
                          fontSize:11,fontWeight:500,
                          color:stage.color,
                          background:stage.bg,
                          padding:'4px 10px',
                          borderRadius:14,
                          fontFamily:'JetBrains Mono,SF Mono,monospace',
                          letterSpacing:-0.1,
                        }}>{t}</span>
                      ))}
                    </div>
                    {/* Why it matters */}
                    <div style={{
                      fontSize:12,lineHeight:1.6,color:'#666',
                      borderLeft:`2px solid ${stage.color}30`,
                      paddingLeft:12,
                      fontStyle:'italic',
                    }}>
                      <strong style={{color:stage.color,fontStyle:'normal',fontWeight:700,marginRight:6}}>WHY IT MATTERS:</strong>
                      {stage.why}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>

          {/* Summary banner */}
          <div style={{
            marginTop:36,
            padding:'24px 28px',
            background:'linear-gradient(135deg,#0F6E56 0%,#25D366 100%)',
            borderRadius:14,
            color:'#fff',
            boxShadow:'0 4px 24px rgba(15,110,86,0.15)',
          }}>
            <div style={{fontSize:11,fontWeight:800,letterSpacing:2,opacity:0.85,marginBottom:8,textTransform:'uppercase'}}>
              <i className="fas fa-circle-info" style={{marginRight:6}}/>so to be clear
            </div>
            <div style={{fontSize:18,fontWeight:700,lineHeight:1.5,marginBottom:12,letterSpacing:-0.3}}>
              Out of 7 stages in the pipeline, only ONE involves an LLM call.
            </div>
            <div style={{fontSize:14,lineHeight:1.7,opacity:0.95}}>
              The other six are hand-built systems: cryptographic hashing, zip-archive parsing, stopword tokenization, ranked SQL retrieval, fuzzy folder matching, and Drive API permission management. The LLM is one tool in a real engineering stack — not the entire product.
            </div>
          </div>

          {/* Plus the analytics layer */}
          <div style={{marginTop:32,padding:'24px 28px',background:'#fff',border:'1px solid #e6e4dc',borderRadius:14}}>
            <div style={{fontSize:11,fontWeight:800,letterSpacing:2,color:'#7F77DD',marginBottom:8,textTransform:'uppercase'}}>
              <i className="fas fa-chart-bar" style={{marginRight:6}}/>plus a real analytics layer
            </div>
            <h3 style={{fontSize:20,fontWeight:800,marginBottom:10,letterSpacing:-0.3}}>Your dashboard speaks SQL, not vibes.</h3>
            <p style={{fontSize:14,lineHeight:1.7,color:'#555',marginBottom:16}}>
              Every stat on the Reyna dashboard comes from a real aggregation query. Subject distribution, top contributors, storage breakdown, group activity — all calculated in SQL on every page load.
            </p>
            <div style={{display:'grid',gridTemplateColumns:'repeat(2,1fr)',gap:10}}>
              {[
                {icon:'fa-chart-pie',label:'Subject distribution',q:'GROUP BY subject'},
                {icon:'fa-trophy',label:'Top 5 contributors',q:'ORDER BY count DESC LIMIT 5'},
                {icon:'fa-database',label:'Storage breakdown',q:'SUM(file_size) by group'},
                {icon:'fa-clock-rotate-left',label:'Activity over time',q:'GROUP BY date(created_at)'},
              ].map((a,i)=>(
                <div key={i} style={{display:'flex',alignItems:'center',gap:10,padding:'10px 14px',background:'#fafafa',border:'1px solid #f0eee6',borderRadius:8}}>
                  <i className={`fas ${a.icon}`} style={{fontSize:14,color:'#7F77DD'}}/>
                  <div style={{flex:1,minWidth:0}}>
                    <div style={{fontSize:13,fontWeight:600,color:'#1a1a1a'}}>{a.label}</div>
                    <code style={{fontSize:10,color:'#888',fontFamily:'JetBrains Mono,monospace'}}>{a.q}</code>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Position recap */}
          <div style={{marginTop:36,textAlign:'center'}}>
            <div style={{fontSize:11,fontWeight:800,color:'#888',letterSpacing:2,textTransform:'uppercase',marginBottom:14}}>
              <i className="fas fa-bullseye" style={{marginRight:6}}/>the bottom line
            </div>
            <p style={{fontSize:18,lineHeight:1.6,color:'#1a1a1a',maxWidth:680,margin:'0 auto',fontWeight:500,letterSpacing:-0.2}}>
              Reyna is one place that <strong style={{color:'#25D366'}}>captures</strong>, <strong style={{color:'#25D366'}}>understands</strong>, <strong style={{color:'#25D366'}}>organises</strong>, <strong style={{color:'#25D366'}}>retrieves</strong>, and <strong style={{color:'#25D366'}}>answers</strong> from every file your group ever shared.
            </p>
            <p style={{fontSize:14,lineHeight:1.6,color:'#666',maxWidth:600,margin:'18px auto 0'}}>
              No more digging through chat history. No more re-uploading to Drive. No more typing answers from notes you can't find. <strong style={{color:'#1a1a1a'}}>Just ask. Reyna handles the rest.</strong>
            </p>
          </div>
        </div>
      </div>

      <div className="rl-divider"/>

      {/* ═══ S7: TECH ARCHITECTURE ═══ */}
      <div id="architecture" style={{background:'#111'}}>
        <div ref={techRef} className={`rl-s ${rv(techVis)}`} style={{maxWidth:900}}>
          <div style={{fontSize:12,fontWeight:700,color:'#25D366',letterSpacing:2.5,textTransform:'uppercase',marginBottom:16}}><i className="fas fa-microchip" style={{marginRight:6}}/>Under the hood</div>
          <h2 style={{fontSize:'clamp(24px,3.5vw,36px)',fontWeight:800,lineHeight:1.2,marginBottom:12,letterSpacing:-1,color:'#fff'}}>Built for real-world scale.</h2>
          <p style={{fontSize:17,lineHeight:1.8,color:'#888',marginBottom:36}}>Not a wrapper around ChatGPT. A purpose-built agentic pipeline with hand-rolled retrieval, parsing, and dedup — backed by a swappable LLM provider.</p>
          <div className="rl-techg" style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:14}}>
            {[
              {icon:'fa-brands fa-whatsapp',t:'WhatsApp / Baileys',d:'Real-time message capture, file download, reaction tracking, wake-word session continuation.',c:'#25D366'},
              {icon:'fa-server',t:'Go backend',d:'Single binary. SQLite + stdlib HTTP. Hash dedup, ranked SQL retrieval, zip-archive parsing — zero ORMs, zero deps.',c:'#0ea5e9'},
              {icon:'fa-brain',t:'Gemini 2.5 Flash',d:'Inline document blocks for PDFs. Provider-agnostic interface — Claude, Grok, or OpenAI swappable via env var.',c:'#7F77DD'},
              {icon:'fa-brands fa-google-drive',t:'Google Drive API',d:'OAuth 2.0. Folder CRUD. Auto-public link permissions. Eventual-consistency-aware classification.',c:'#FBBC04'},
              {icon:'fa-brands fa-react',t:'React + Vite',d:'1.2k LOC, no component library. Conversational chat threads, markdown rendering, auto-grow textareas.',c:'#61DAFB'},
              {icon:'fa-database',t:'SQLite',d:'Files, hashes, group settings, extracted content, sessions. Partial UNIQUE indexes for dedup. Zero migration hell.',c:'#003B57'},
            ].map((tech,i)=>(
              <div key={i} style={{background:'#1a1a1a',border:'1px solid #333',borderRadius:12,padding:20,transition:'border-color .2s'}}>
                <i className={`fas ${tech.icon}`} style={{fontSize:20,color:tech.c,marginBottom:10,display:'block'}}/>
                <div style={{fontSize:14,fontWeight:700,color:'#fff',marginBottom:4}}>{tech.t}</div>
                <div style={{fontSize:12,color:'#888',lineHeight:1.6}}>{tech.d}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ═══ S8: CTA / WAITLIST ═══ */}
      <section id="waitlist" ref={ctaRef} className={rv(ctaVis)} style={{background:'#111',padding:'80px 24px',textAlign:'center'}}>
        <h2 style={{fontSize:'clamp(28px,4.5vw,44px)',fontWeight:900,lineHeight:1.2,marginBottom:16,color:'#fff',letterSpacing:-1}}>Your files deserve better than vanishing into chat history.</h2>
        <p style={{fontSize:17,color:'#888',maxWidth:500,margin:'0 auto 32px',lineHeight:1.7}}>
          Five autonomous agents. Content extraction.<br/>Conversational retrieval. Notes Q&A.<br/><strong style={{color:'#25D366'}}>That's Reyna.</strong>
        </p>
        {!joined?(
          <div style={{maxWidth:440,margin:'0 auto',display:'flex',gap:8}}>
            <input type="text" value={contact} onChange={e=>setContact(e.target.value)} onKeyDown={e=>e.key==='Enter'&&joinWaitlist()} placeholder="Your email or phone number"
              style={{flex:1,background:'#1a1a1a',border:'1px solid #333',borderRadius:8,padding:'14px 18px',color:'#fff',fontSize:14,outline:'none',fontFamily:'Inter,sans-serif'}}/>
            <button onClick={joinWaitlist} style={{fontSize:14,fontWeight:700,color:'#111',background:'#25D366',padding:'14px 24px',borderRadius:8,border:'none',cursor:'pointer',whiteSpace:'nowrap',display:'flex',alignItems:'center',gap:6}}>
              Get Early Access <i className="fas fa-arrow-right" style={{fontSize:11}}/>
            </button>
          </div>
        ):(
          <div style={{fontSize:18,color:'#25D366',fontWeight:700,display:'flex',alignItems:'center',justifyContent:'center',gap:8}}><i className="fas fa-check-circle"/> You're in. We'll reach out soon.</div>
        )}
        <p style={{fontSize:12,color:'#555',marginTop:16}}>Free core features. Open source. Your data stays in YOUR Drive.</p>
      </section>

      {/* Footer */}
      <footer style={{borderTop:'1px solid #222',background:'#111',padding:'24px 32px',textAlign:'center'}}>
        <p style={{fontSize:13,color:'#999',marginBottom:6}}>
          <span style={{fontWeight:800,color:'#fff',letterSpacing:1}}><i className="fas fa-crown" style={{color:'#25D366',fontSize:11,marginRight:4}}/>REYNA</span>
          <span style={{margin:'0 12px',color:'#333'}}>|</span>
          <a href="#" style={{color:'#666'}}><i className="fab fa-github" style={{marginRight:4}}/>GitHub</a> · <a href="#" style={{color:'#666'}}><i className="fab fa-twitter" style={{marginRight:4}}/>Twitter</a>
        </p>
        <p style={{fontSize:12,color:'#555'}}>Built for people who are tired of losing files in group chats.</p>
      </footer>
    </div>
  )
}
