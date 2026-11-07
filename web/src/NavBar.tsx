export default function NavBar() {
  const rektion = window.location.pathname.startsWith('/rektion')
  return <header className="topbar">
    <a className="brand" href="/" aria-label="Artikel home"><span className="brand-mark">A</span><span>Artikel<span className="brand-dot">.</span></span></a>
    <nav className="main-nav" aria-label="Main navigation">
      <a href="/" className={!rektion ? 'active' : ''} aria-current={!rektion ? 'page' : undefined}>Artikel</a>
      <a href="/rektion" className={rektion ? 'active' : ''} aria-current={rektion ? 'page' : undefined}>Rektion</a>
    </nav>
    <span className="top-note">A little German, every day</span>
  </header>
}
