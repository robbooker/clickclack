export function css() {
  return `
:root{
  --ink:#1a0e0a;
  --text:#241914;
  --muted:#7a6e62;
  --subtle:#a59a8e;
  --bg:#fbf6ee;
  --paper:#ffffff;
  --paper-soft:#fdf9f1;
  --shell:#f5e7d3;
  --shell-deep:#ead7bb;
  --sand:#f3e7d3;
  --coral:#ff5b4a;
  --coral-deep:#d33a2c;
  --coral-soft:rgba(255,91,74,.10);
  --brine:#0e8c8c;
  --brine-soft:rgba(14,140,140,.12);
  --bubble:#9fd9d9;
  --claw-yellow:#f6c84c;
  --line:#ead7bb;
  --line-soft:#f1e4cf;
  --pill-border:#e2d2b6;
  --code-bg:#1a0e0a;
  --code-fg:#fceedd;
  --code-inline-fg:#241914;
  --code-border:#2a1c14;
  --shadow-card:0 4px 20px rgba(60,28,12,.10);
  --scrollbar:#d8c2a3;
  --hl-keyword:#ffb3a8;
  --hl-string:#ffd17a;
  --hl-number:#9fd9d9;
  --hl-comment:#a59a8e;
  --hl-flag:#f6c84c;
  --hl-meta:#ff8aa0;
  --hl-prompt:#7a6e62;
  --accent:var(--coral);
  --accent-soft:var(--coral-soft);
  --accent-strong:var(--coral-deep);
}
:root[data-theme="dark"]{
  --ink:#fceedd;
  --text:#dccab2;
  --muted:#a59a8e;
  --subtle:#7a6e62;
  --bg:#0c0805;
  --paper:#15100b;
  --paper-soft:#1a130d;
  --shell:#241712;
  --shell-deep:#321f15;
  --sand:#2a1c14;
  --coral:#ff7967;
  --coral-deep:#ff5b4a;
  --coral-soft:rgba(255,121,103,.16);
  --brine:#3fbcbc;
  --brine-soft:rgba(63,188,188,.18);
  --bubble:#3fbcbc;
  --claw-yellow:#f6c84c;
  --line:#2c1f15;
  --line-soft:#221710;
  --pill-border:#3a2a1d;
  --code-bg:#06030a;
  --code-fg:#fceedd;
  --code-inline-fg:#fceedd;
  --code-border:#2a1c14;
  --shadow-card:0 4px 24px rgba(0,0,0,.55);
  --scrollbar:#4a3a2a;
  --hl-keyword:#ff9b8a;
  --hl-string:#f6c84c;
  --hl-number:#9fd9d9;
  --hl-comment:#7a6e62;
  --hl-flag:#f6c84c;
  --hl-meta:#ff8aa0;
  --hl-prompt:#a59a8e;
}
:root{color-scheme:light}
:root[data-theme="dark"]{color-scheme:dark}
*{box-sizing:border-box}
html{scroll-behavior:smooth;scroll-padding-top:24px}
body{
  margin:0;
  background:var(--bg);
  color:var(--text);
  font-family:"Inter",ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif;
  line-height:1.65;
  overflow-x:hidden;
  -webkit-font-smoothing:antialiased;
  font-feature-settings:"cv02","cv03","cv04","cv11","ss01";
  background-image:
    radial-gradient(circle at 1px 1px, var(--shell-deep) 1px, transparent 0);
  background-size:22px 22px;
  background-position:0 0;
  background-attachment:fixed;
  transition:background-color .18s,color .18s;
}
body.home{background-image:radial-gradient(circle at 1px 1px, var(--shell-deep) 1px, transparent 0),radial-gradient(ellipse at 80% -10%, var(--coral-soft), transparent 55%);background-size:22px 22px,100% 70vh;background-repeat:repeat,no-repeat}
::selection{background:var(--coral);color:#fff}
a{color:var(--coral-deep);text-decoration:none;transition:color .12s}
a:hover{text-decoration:underline;text-underline-offset:.2em}
:root[data-theme="dark"] a{color:var(--coral)}
.shell{display:grid;grid-template-columns:268px minmax(0,1fr);min-height:100vh}
.sidebar{position:sticky;top:0;height:100vh;overflow:auto;padding:22px 20px;background:var(--paper);border-right:1px solid var(--line);scrollbar-width:thin;scrollbar-color:var(--line) transparent;transition:background-color .18s,border-color .18s;display:flex;flex-direction:column;gap:18px}
.sidebar::-webkit-scrollbar{width:6px}
.sidebar::-webkit-scrollbar-thumb{background:var(--scrollbar);border-radius:6px}
.sidebar-head{display:flex;align-items:center;gap:10px;margin:0}
.brand{display:flex;align-items:center;gap:11px;color:var(--ink);text-decoration:none;flex:1;min-width:0}
.brand:hover{text-decoration:none}
.brand .mark{display:block;flex:0 0 32px;width:32px;height:32px;line-height:0;filter:drop-shadow(0 1px 0 rgba(60,28,12,.12))}
.brand .mark svg{width:32px;height:32px;display:block}
.brand-text{display:flex;flex-direction:column;min-width:0}
.brand strong{display:block;font:700 1.06rem/1.05 "Fraunces",ui-serif,Georgia,serif;letter-spacing:-.01em;color:var(--ink)}
.brand small{display:block;color:var(--muted);font-size:.7rem;margin-top:2px;font-weight:500;letter-spacing:.02em;text-transform:lowercase}
.theme-toggle{display:inline-flex;align-items:center;justify-content:center;flex:0 0 auto;width:34px;height:34px;border-radius:9px;border:1px solid var(--line);background:var(--paper-soft);color:var(--muted);cursor:pointer;padding:0;transition:border-color .15s,color .15s,background-color .15s,transform .12s}
.theme-toggle:hover{border-color:var(--ink);color:var(--ink)}
.theme-toggle:active{transform:scale(.94)}
.theme-toggle svg{width:16px;height:16px;display:block}
.theme-icon-sun{display:none}
:root[data-theme="dark"] .theme-icon-sun{display:block}
:root[data-theme="dark"] .theme-icon-moon{display:none}
.search{display:block;margin:0}
.search span{display:block;color:var(--muted);font-size:.66rem;font-weight:600;text-transform:uppercase;letter-spacing:.08em;margin-bottom:6px}
.search input{width:100%;border:1px solid var(--line);background:var(--paper-soft);border-radius:9px;padding:9px 12px;font:inherit;font-size:.9rem;color:var(--text);outline:none;transition:border-color .15s,box-shadow .15s,background-color .18s}
.search input:focus{border-color:var(--coral);box-shadow:0 0 0 3px var(--coral-soft);background:var(--paper)}
nav{flex:1 1 auto;min-height:0}
nav section{margin:0 0 16px}
nav h2{font-size:.66rem;color:var(--muted);text-transform:uppercase;letter-spacing:.1em;margin:0 0 4px;font-weight:700}
.nav-link{display:block;color:var(--text);text-decoration:none;border-radius:7px;padding:5px 10px;margin:1px 0;font-size:.9rem;line-height:1.4;transition:background .12s,color .12s}
.nav-link:hover{background:var(--shell);color:var(--ink);text-decoration:none}
.nav-link.active{background:var(--coral-soft);color:var(--coral-deep);font-weight:600}
:root[data-theme="dark"] .nav-link.active{color:var(--coral)}
.sidebar-foot{margin-top:auto;padding-top:14px;border-top:1px dashed var(--line)}
.foot-link{display:block;color:var(--muted);font-size:.74rem;text-decoration:none;font-family:"JetBrains Mono","SF Mono",ui-monospace,monospace;letter-spacing:-.01em}
.foot-link:hover{color:var(--ink);text-decoration:none}
main{min-width:0;padding:32px clamp(20px,4.5vw,60px) 96px;max-width:1200px;margin:0 auto;width:100%}
.hero{display:flex;align-items:flex-end;justify-content:space-between;gap:22px;border-bottom:1px solid var(--line);padding:8px 0 22px;margin-bottom:8px;flex-wrap:wrap}
.hero-text{min-width:0;flex:1 1 320px}
.eyebrow{margin:0 0 8px;color:var(--brine);font-weight:700;text-transform:uppercase;letter-spacing:.14em;font-size:.7rem}
.hero h1{font:700 2.4rem/1.05 "Fraunces",ui-serif,Georgia,serif;letter-spacing:-.02em;margin:0;color:var(--ink)}
.hero-meta{display:flex;gap:8px;flex:0 0 auto;flex-wrap:wrap}
.repo,.edit,.btn-ghost{border:1px solid var(--line);color:var(--text);text-decoration:none;border-radius:8px;padding:6px 12px;font-weight:500;font-size:.83rem;background:var(--paper);transition:border-color .15s,color .15s,background .15s,transform .1s}
.repo:hover,.edit:hover,.btn-ghost:hover{border-color:var(--ink);color:var(--ink);text-decoration:none;transform:translateY(-1px)}
.edit{color:var(--muted)}
.home-hero{padding:14px 0 32px;margin-bottom:8px;border-bottom:1px solid var(--line);position:relative}
.home-hero-mark{position:absolute;top:0;right:0;width:160px;max-width:36vw;opacity:.95;pointer-events:none;transform:rotate(-6deg);transform-origin:top right}
.home-hero-mark svg{width:100%;height:auto;display:block}
.home-hero-mark::after{content:"";position:absolute;inset:auto -10% -20% 30%;height:14px;border-radius:50%;background:radial-gradient(ellipse at center, rgba(60,28,12,.18), transparent 70%);filter:blur(2px)}
.home-hero h1{font:700 3.4rem/1.0 "Fraunces",ui-serif,Georgia,serif;letter-spacing:-.025em;margin:0 0 .35em;color:var(--ink);max-width:14ch}
.home-hero h1 em{font-style:italic;color:var(--coral-deep);font-variation-settings:"opsz" 144}
:root[data-theme="dark"] .home-hero h1 em{color:var(--coral)}
.home-hero .lede{font-size:1.15rem;line-height:1.55;color:var(--text);margin:0 0 1.2em;max-width:60ch}
.home-cta{display:flex;flex-wrap:wrap;gap:10px;align-items:center;margin:0 0 18px}
.home-cta .btn{display:inline-flex;align-items:center;gap:7px;border-radius:9px;padding:11px 17px;font-weight:600;font-size:.92rem;text-decoration:none;transition:background .15s,border-color .15s,color .15s,transform .12s,box-shadow .15s}
.home-cta .btn-primary{background:var(--coral);color:#fff;border:1px solid var(--coral);box-shadow:0 1px 0 rgba(60,28,12,.18),0 8px 18px -8px var(--coral)}
.home-cta .btn-primary:hover{background:var(--coral-deep);border-color:var(--coral-deep);text-decoration:none;transform:translateY(-1px)}
:root[data-theme="dark"] .home-cta .btn-primary{background:var(--coral);border-color:var(--coral);color:#0c0805;box-shadow:0 0 0 1px rgba(255,255,255,.12),0 10px 22px -10px var(--coral)}
:root[data-theme="dark"] .home-cta .btn-primary:hover{background:#ff9a8b;border-color:#ff9a8b;color:#0c0805}
.home-cta .btn-ghost{padding:11px 17px}
.home-install{display:flex;align-items:center;gap:12px;background:var(--code-bg);color:var(--code-fg);border-radius:9px;padding:10px 10px 10px 16px;font:500 .9rem/1.2 "JetBrains Mono","SF Mono",ui-monospace,monospace;max-width:36em;border:1px solid var(--code-border);box-shadow:0 6px 18px -10px rgba(60,28,12,.4)}
.home-install .prompt{color:var(--coral);user-select:none;flex:0 0 auto}
.home-install code{flex:1;background:transparent;border:0;color:var(--code-fg);font:inherit;padding:0;white-space:pre;overflow:hidden;text-overflow:ellipsis}
.home-install .copy{flex:0 0 auto;background:rgba(255,255,255,.08);color:var(--code-fg);border:1px solid rgba(255,255,255,.16);border-radius:6px;padding:5px 11px;font:600 .72rem/1 "Inter",sans-serif;cursor:pointer;letter-spacing:.04em;transition:background .15s,border-color .15s}
.home-install .copy:hover{background:rgba(255,255,255,.16)}
.home-install .copy.copied{background:var(--coral);border-color:var(--coral)}
.home-services{display:flex;flex-wrap:wrap;gap:6px;margin:6px 0 18px}
.home-services span{display:inline-flex;align-items:center;gap:5px;padding:4px 11px;border:1px solid var(--pill-border);border-radius:999px;font-size:.78rem;color:var(--ink);background:var(--paper);box-shadow:0 1px 0 rgba(60,28,12,.04)}
.home-services span::before{content:"";display:inline-block;width:6px;height:6px;border-radius:50%;background:var(--coral)}
.home-services span:nth-child(3n)::before{background:var(--brine)}
.home-services span:nth-child(5n)::before{background:var(--claw-yellow)}
.muted{color:var(--muted);font-size:.92rem}
.muted a{color:var(--brine);font-weight:500}
:root[data-theme="dark"] .muted a{color:var(--brine)}
.muted a:hover{color:var(--coral-deep)}
.doc-grid{display:grid;grid-template-columns:minmax(0,1fr);gap:48px;margin-top:24px}
.doc-grid-home{margin-top:8px}
@media(min-width:1180px){.doc-grid{grid-template-columns:minmax(0,72ch) 200px;justify-content:start}.doc-grid-home{grid-template-columns:minmax(0,76ch);justify-content:start}}
.doc{min-width:0;max-width:72ch;overflow-wrap:break-word}
.doc-home{max-width:76ch}
.doc h1{font:700 2.6rem/1.05 "Fraunces",ui-serif,Georgia,serif;letter-spacing:-.02em;margin:0 0 .4em;color:var(--ink)}
body:not(.home) .doc>h1:first-child{display:none}
.doc h2{font:600 1.5rem/1.2 "Fraunces",ui-serif,Georgia,serif;margin:2em 0 .5em;letter-spacing:-.01em;color:var(--ink);position:relative}
.doc h3{font-size:1.12rem;margin:1.7em 0 .35em;position:relative;font-weight:600;color:var(--ink);letter-spacing:-.005em}
.doc h4{font-size:.98rem;margin:1.4em 0 .25em;color:var(--ink);position:relative;font-weight:600}
.doc h2:first-child,.doc h3:first-child,.doc h4:first-child{margin-top:.2em}
.doc :is(h2,h3,h4) .anchor{position:absolute;left:-1.05em;top:0;color:var(--subtle);opacity:0;text-decoration:none;font-weight:400;padding-right:.3em;transition:opacity .12s,color .12s}
.doc :is(h2,h3,h4):hover .anchor{opacity:.7}
.doc :is(h2,h3,h4) .anchor:hover{opacity:1;color:var(--coral);text-decoration:none}
.doc p{margin:0 0 1.05em}
.doc ul,.doc ol{padding-left:1.3rem;margin:0 0 1.15em}
.doc li{margin:.25em 0}
.doc li>p{margin:0 0 .4em}
.doc ul li::marker{color:var(--coral)}
.doc strong{font-weight:700;color:var(--ink)}
.doc em{font-style:italic;color:var(--ink)}
.doc code{font-family:"JetBrains Mono","SF Mono",ui-monospace,monospace;font-size:.84em;background:var(--shell);border:1px solid var(--line);border-radius:5px;padding:.08em .35em;color:var(--code-inline-fg)}
.doc pre{position:relative;overflow:auto;background:var(--code-bg);color:var(--code-fg);border-radius:10px;padding:14px 18px;margin:1.3em 0;font-size:.85em;line-height:1.6;scrollbar-width:thin;scrollbar-color:#4a3a2a transparent;border:1px solid var(--code-border);box-shadow:0 6px 22px -16px rgba(60,28,12,.5)}
.doc pre::-webkit-scrollbar{height:8px;width:8px}
.doc pre::-webkit-scrollbar-thumb{background:#4a3a2a;border-radius:8px}
.doc pre code{display:block;background:transparent;border:0;color:inherit;padding:0;font-size:1em;white-space:pre}
.doc pre .copy{position:absolute;top:8px;right:8px;background:rgba(255,255,255,.06);color:var(--code-fg);border:1px solid rgba(255,255,255,.14);border-radius:6px;padding:3px 9px;font:600 .7rem/1 "Inter",sans-serif;cursor:pointer;opacity:0;letter-spacing:.04em;transition:opacity .15s,background .15s,border-color .15s}
.doc pre:hover .copy,.doc pre .copy:focus{opacity:1}
.doc pre .copy:hover{background:rgba(255,255,255,.12)}
.doc pre .copy.copied{background:var(--coral);border-color:var(--coral);opacity:1}
.doc pre .hl-c{color:var(--hl-comment);font-style:italic}
.doc pre .hl-s{color:var(--hl-string)}
.doc pre .hl-n{color:var(--hl-number)}
.doc pre .hl-k{color:var(--hl-keyword);font-weight:600}
.doc pre .hl-f{color:var(--hl-flag)}
.doc pre .hl-m{color:var(--hl-meta);font-weight:600}
.doc pre .hl-p{color:var(--hl-prompt);user-select:none}
.doc pre .hl-cmd{color:var(--hl-keyword);font-weight:600}
.doc blockquote{margin:1.4em 0;padding:12px 16px;border-left:3px solid var(--coral);background:var(--coral-soft);border-radius:0 9px 9px 0;color:var(--text)}
.doc blockquote p:last-child{margin-bottom:0}
.doc table{width:100%;border-collapse:collapse;margin:1.2em 0;font-size:.92em;border:1px solid var(--line);border-radius:9px;overflow:hidden}
.doc th,.doc td{border-bottom:1px solid var(--line);padding:9px 12px;text-align:left;vertical-align:top}
.doc tr:last-child td{border-bottom:0}
.doc th{font-weight:700;color:var(--ink);background:var(--shell);border-bottom:1px solid var(--line);font-size:.86em;letter-spacing:.01em;text-transform:uppercase}
.doc hr{border:0;border-top:1px dashed var(--line);margin:2.2em 0}
.toc{position:sticky;top:24px;align-self:start;font-size:.84rem;padding-left:14px;border-left:1px solid var(--line);max-height:calc(100vh - 48px);overflow:auto;scrollbar-width:thin;scrollbar-color:var(--line) transparent}
.toc::-webkit-scrollbar{width:5px}
.toc::-webkit-scrollbar-thumb{background:var(--line);border-radius:5px}
.toc h2{font-size:.66rem;color:var(--muted);text-transform:uppercase;letter-spacing:.1em;margin:0 0 10px;font-weight:700}
.toc a{display:block;color:var(--muted);text-decoration:none;padding:4px 0 4px 10px;line-height:1.35;border-left:2px solid transparent;margin-left:-12px;transition:color .12s,border-color .12s}
.toc a:hover{color:var(--ink);text-decoration:none}
.toc a.active{color:var(--coral-deep);border-left-color:var(--coral);font-weight:600}
:root[data-theme="dark"] .toc a.active{color:var(--coral)}
.toc-l3{padding-left:22px!important;font-size:.94em}
@media(max-width:1179px){.toc{display:none}}
.page-nav{display:grid;grid-template-columns:1fr 1fr;gap:14px;margin-top:48px;border-top:1px dashed var(--line);padding-top:22px}
.page-nav>a{display:block;border:1px solid var(--line);background:var(--paper);border-radius:10px;padding:14px 16px;text-decoration:none;color:var(--text);transition:border-color .15s,transform .15s,box-shadow .15s,background-color .18s}
.page-nav>a:hover{border-color:var(--coral);text-decoration:none;color:var(--ink);transform:translateY(-1px);box-shadow:var(--shadow-card)}
.page-nav small{display:block;color:var(--muted);font-size:.68rem;text-transform:uppercase;letter-spacing:.1em;margin-bottom:4px;font-weight:700}
.page-nav small::before{content:"〕 ";color:var(--coral);font-style:normal}
.page-nav-next small::before{content:""}
.page-nav-next small::after{content:" 〔";color:var(--coral)}
.page-nav span{display:block;font:600 1rem/1.25 "Fraunces",ui-serif,Georgia,serif;color:var(--ink);letter-spacing:-.005em}
.page-nav-prev{text-align:left}
.page-nav-next{text-align:right;grid-column:2}
.page-nav-prev:only-child{grid-column:1}
.nav-toggle{display:none;position:fixed;top:14px;right:14px;top:calc(14px + env(safe-area-inset-top, 0px));right:calc(14px + env(safe-area-inset-right, 0px));z-index:20;width:40px;height:40px;border-radius:10px;background:var(--paper);border:1px solid var(--line);color:var(--ink);cursor:pointer;padding:10px 9px;flex-direction:column;align-items:stretch;justify-content:space-between;box-shadow:var(--shadow-card)}
.nav-toggle span{display:block;width:100%;height:2px;flex:0 0 2px;background:currentColor;border-radius:2px;transition:transform .2s,opacity .2s}
.nav-toggle[aria-expanded="true"] span:nth-child(1){transform:translateY(8px) rotate(45deg)}
.nav-toggle[aria-expanded="true"] span:nth-child(2){opacity:0}
.nav-toggle[aria-expanded="true"] span:nth-child(3){transform:translateY(-8px) rotate(-45deg)}
@media(max-width:900px){
  .shell{display:block}
  .sidebar{position:fixed;inset:0 30% 0 0;max-width:320px;height:100vh;z-index:15;transform:translateX(-100%);transition:transform .25s ease,background-color .18s,border-color .18s;box-shadow:0 18px 40px rgba(60,28,12,.22);background:var(--paper);pointer-events:none}
  .sidebar.open{transform:translateX(0);pointer-events:auto}
  .nav-toggle{display:flex}
  main{padding:64px 18px 64px}
  .hero{padding-top:6px}
  .hero h1{font-size:1.85rem}
  .home-hero h1{font-size:2.5rem;max-width:none}
  .home-hero-mark{width:110px;opacity:.7}
  .doc h1{font-size:2.05rem}
  .hero-meta{width:100%;justify-content:flex-start}
  .home-hero{padding-top:8px}
  .doc{padding:0}
  .doc-grid{margin-top:18px;gap:24px}
  .doc :is(h2,h3,h4) .anchor{display:none}
}
@media(max-width:520px){
  main{padding:60px 14px 56px}
  .doc pre{margin-left:-14px;margin-right:-14px;border-radius:0;border-left:0;border-right:0}
  .home-install{flex-wrap:wrap}
  .home-hero-mark{display:none}
}
`;
}

export function js() {
  return `
const themeRoot=document.documentElement;
function applyTheme(mode){themeRoot.dataset.theme=mode;document.querySelectorAll('[data-theme-toggle]').forEach(b=>b.setAttribute('aria-pressed',mode==='dark'?'true':'false'))}
function storedTheme(){try{return localStorage.getItem('theme')}catch(e){return null}}
function persistTheme(mode){try{localStorage.setItem('theme',mode)}catch(e){}}
applyTheme(themeRoot.dataset.theme==='dark'?'dark':'light');
document.querySelectorAll('[data-theme-toggle]').forEach(btn=>{btn.addEventListener('click',()=>{const next=themeRoot.dataset.theme==='dark'?'light':'dark';applyTheme(next);persistTheme(next)})});
const systemDark=window.matchMedia&&matchMedia('(prefers-color-scheme: dark)');
function onSystemChange(e){if(storedTheme())return;applyTheme(e.matches?'dark':'light')}
if(systemDark){if(systemDark.addEventListener)systemDark.addEventListener('change',onSystemChange);else if(systemDark.addListener)systemDark.addListener(onSystemChange)}
const sidebar=document.querySelector('.sidebar');
const toggle=document.querySelector('.nav-toggle');
const mobileNav=window.matchMedia('(max-width: 900px)');
const sidebarFocusable='a[href],button,input,select,textarea,[tabindex]';
function setSidebarFocusable(enabled){
  sidebar?.querySelectorAll(sidebarFocusable).forEach((el)=>{
    if(enabled){
      if(el.dataset.sidebarTabindex!==undefined){
        if(el.dataset.sidebarTabindex)el.setAttribute('tabindex',el.dataset.sidebarTabindex);
        else el.removeAttribute('tabindex');
        delete el.dataset.sidebarTabindex;
      }
    }else if(el.dataset.sidebarTabindex===undefined){
      el.dataset.sidebarTabindex=el.getAttribute('tabindex')??'';
      el.setAttribute('tabindex','-1');
    }
  });
}
function setSidebarOpen(open){
  if(!sidebar||!toggle)return;
  sidebar.classList.toggle('open',open);
  toggle.setAttribute('aria-expanded',open?'true':'false');
  if(mobileNav.matches){
    sidebar.inert=!open;
    if(open)sidebar.removeAttribute('aria-hidden');
    else sidebar.setAttribute('aria-hidden','true');
    setSidebarFocusable(open);
  }else{
    sidebar.inert=false;
    sidebar.removeAttribute('aria-hidden');
    setSidebarFocusable(true);
  }
}
setSidebarOpen(false);
toggle?.addEventListener('click',()=>setSidebarOpen(!sidebar?.classList.contains('open')));
document.addEventListener('click',(e)=>{if(!sidebar?.classList.contains('open'))return;if(sidebar.contains(e.target)||toggle?.contains(e.target))return;setSidebarOpen(false)});
document.addEventListener('keydown',(e)=>{if(e.key==='Escape')setSidebarOpen(false)});
const syncSidebarForViewport=()=>setSidebarOpen(sidebar?.classList.contains('open')??false);
if(mobileNav.addEventListener)mobileNav.addEventListener('change',syncSidebarForViewport);
else mobileNav.addListener?.(syncSidebarForViewport);
const input=document.getElementById('doc-search');
input?.addEventListener('input',()=>{const q=input.value.trim().toLowerCase();document.querySelectorAll('nav section').forEach(sec=>{let any=false;sec.querySelectorAll('.nav-link').forEach(a=>{const m=!q||a.textContent.toLowerCase().includes(q);a.style.display=m?'block':'none';if(m)any=true});sec.style.display=any?'block':'none'})});
function attachCopy(target,getText){const btn=document.createElement('button');btn.type='button';btn.className='copy';btn.textContent='Copy';btn.addEventListener('click',async()=>{try{await navigator.clipboard.writeText(getText());btn.textContent='Copied';btn.classList.add('copied');setTimeout(()=>{btn.textContent='Copy';btn.classList.remove('copied')},1400)}catch{btn.textContent='Failed';setTimeout(()=>{btn.textContent='Copy'},1400)}});target.appendChild(btn)}
document.querySelectorAll('.doc pre').forEach(pre=>attachCopy(pre,()=>pre.querySelector('code')?.textContent??''));
document.querySelectorAll('.home-install').forEach(el=>attachCopy(el,()=>el.querySelector('code')?.textContent??''));
const tocLinks=document.querySelectorAll('.toc a');
if(tocLinks.length){const map=new Map();tocLinks.forEach(a=>{const id=a.getAttribute('href').slice(1);const el=document.getElementById(id);if(el)map.set(el,a)});const setActive=l=>{tocLinks.forEach(x=>x.classList.remove('active'));l.classList.add('active')};const obs=new IntersectionObserver(entries=>{const visible=entries.filter(e=>e.isIntersecting).sort((a,b)=>a.boundingClientRect.top-b.boundingClientRect.top);if(visible.length){const link=map.get(visible[0].target);if(link)setActive(link)}},{rootMargin:'-15% 0px -65% 0px',threshold:0});map.forEach((_,el)=>obs.observe(el))}
`;
}

export function preThemeScript() {
  return `(function(){var s;try{s=localStorage.getItem('theme')}catch(e){}var d=window.matchMedia&&matchMedia('(prefers-color-scheme: dark)').matches;document.documentElement.dataset.theme=s||(d?'dark':'light')})();`;
}

export function themeToggleHtml() {
  return `<button class="theme-toggle" type="button" aria-label="Toggle dark mode" aria-pressed="false" data-theme-toggle>
    <svg class="theme-icon-moon" viewBox="0 0 20 20" aria-hidden="true"><path d="M14.6 12.1A6.5 6.5 0 0 1 7.4 2.7a6.5 6.5 0 1 0 7.2 9.4z" fill="currentColor"/></svg>
    <svg class="theme-icon-sun" viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="3.4" fill="currentColor"/><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><line x1="10" y1="2" x2="10" y2="4"/><line x1="10" y1="16" x2="10" y2="18"/><line x1="2" y1="10" x2="4" y2="10"/><line x1="16" y1="10" x2="18" y2="10"/><line x1="4.2" y1="4.2" x2="5.6" y2="5.6"/><line x1="14.4" y1="14.4" x2="15.8" y2="15.8"/><line x1="4.2" y1="15.8" x2="5.6" y2="14.4"/><line x1="14.4" y1="5.6" x2="15.8" y2="4.2"/></g></svg>
  </button>`;
}

export function faviconSvg() {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="ClickClack">
<defs>
  <linearGradient id="cc-coral" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#ff7967"/>
    <stop offset="1" stop-color="#d33a2c"/>
  </linearGradient>
</defs>
<rect width="64" height="64" rx="14" fill="url(#cc-coral)"/>
<g fill="#fbf6ee" stroke="#1a0e0a" stroke-width="2" stroke-linejoin="round">
  <path d="M22 24 C16 24 12 28 12 34 C12 40 16 44 22 44 L22 39 C19 39 17 37 17 34 C17 31 19 29 22 29 L26 29 L26 26 L24 24 Z"/>
  <path d="M42 24 C48 24 52 28 52 34 C52 40 48 44 42 44 L42 39 C45 39 47 37 47 34 C47 31 45 29 42 29 L38 29 L38 26 L40 24 Z"/>
</g>
<g fill="#1a0e0a">
  <circle cx="20" cy="34" r="2"/>
  <circle cx="44" cy="34" r="2"/>
</g>
<circle cx="32" cy="34" r="3" fill="none" stroke="#9fd9d9" stroke-width="2"/>
<circle cx="32" cy="34" r="1.2" fill="#9fd9d9"/>
</svg>`;
}
