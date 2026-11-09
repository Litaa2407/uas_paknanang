import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const GATEWAY = 'http://localhost:8000';

function roleFromToken(token) {
  try {
    const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(payload)).role || '';
  } catch {
    return '';
  }
}

function App() {
  const [campaigns, setCampaigns] = useState([]);
  const [pendingCampaigns, setPendingCampaigns] = useState([]);
  const [token, setToken] = useState(localStorage.getItem('token') || '');
  const [userRole, setUserRole] = useState(() => roleFromToken(localStorage.getItem('token') || ''));
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState('login');
  const [authError, setAuthError] = useState('');
  const [authLoading, setAuthLoading] = useState(false);
  const [title, setTitle] = useState('');
  const [target, setTarget] = useState('');
  const [events, setEvents] = useState([]);
  const [health, setHealth] = useState('memeriksa');

  async function gql(query) {
    const response = await fetch(`${GATEWAY}/graphql`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ query }),
    });
    return response.json();
  }

  async function loadCampaigns() {
    const result = await gql('{ campaigns { id title description target collected status } }');
    if (result.data?.campaigns) setCampaigns(result.data.campaigns);
  }

  async function loadPendingCampaigns() {
    const result = await gql('{ pendingCampaigns { id title description target collected status } }');
    if (result.errors) {
      alert(result.errors[0].message);
      return;
    }
    setPendingCampaigns(result.data?.pendingCampaigns || []);
  }

  async function auth(path) {
    try {
      setAuthLoading(true);
      setAuthError('');
      const response = await fetch(`${GATEWAY}/auth/${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email,
          password,
          ...(path === 'register' ? { name: email.split('@')[0] } : {}),
        }),
      });
      const raw = await response.text();
      let data;
      try {
        data = JSON.parse(raw);
      } catch {
        data = { error: raw };
      }
      if (!response.ok) {
        setAuthError(data.error || 'Permintaan gagal. Silakan coba lagi.');
        return;
      }
      setToken(data.token);
      setUserRole(data.user?.role || roleFromToken(data.token));
      localStorage.setItem('token', data.token);
    } catch {
      setAuthError('Tidak dapat terhubung ke server. Pastikan aplikasi sedang berjalan.');
    } finally {
      setAuthLoading(false);
    }
  }

  async function createCampaign(event) {
    event.preventDefault();
    const result = await gql(`mutation {
      createCampaign(title:${JSON.stringify(title)}, description:"", target:${Number(target)}) {
        id title status
      }
    }`);
    if (result.errors) {
      alert(result.errors[0].message);
      return;
    }
    setTitle('');
    setTarget('');
    alert('Campaign berhasil dikirim dan menunggu persetujuan Admin sebelum ditampilkan publik.');
    await loadCampaigns();
    if (userRole === 'ADMIN') await loadPendingCampaigns();
  }

  async function approveCampaign(id) {
    const result = await gql(`mutation { approveCampaign(id:${id}) { id title status } }`);
    if (result.errors) {
      alert(result.errors[0].message);
      return;
    }
    await loadPendingCampaigns();
    await loadCampaigns();
  }

  function logout() {
    localStorage.removeItem('token');
    setToken('');
    setUserRole('');
    setPendingCampaigns([]);
  }

  useEffect(() => {
    loadCampaigns();
    fetch(`${GATEWAY}/registry`)
      .then((response) => response.json())
      .then(() => setHealth('online'))
      .catch(() => setHealth('offline'));
    const socket = new WebSocket('ws://localhost:8000/ws/donasi');
    socket.onmessage = (event) => {
      setEvents((current) => [JSON.parse(event.data), ...current].slice(0, 8));
    };
    return () => socket.close();
  }, []);

  useEffect(() => {
    if (token && userRole === 'ADMIN') {
      loadPendingCampaigns();
    } else {
      setPendingCampaigns([]);
    }
  }, [token, userRole]);

  if (!token) {
    return (
      <main className="auth-page">
        <section className="auth-layout">
          <div className="auth-story">
            <a className="brand" href="/" aria-label="Donasi Bersama">
              <span className="brand-mark">d.</span>
              <span>donasi<span className="brand-light">bersama</span></span>
            </a>
            <div className="story-copy">
              <span className="eyebrow"><i /> PLATFORM CROWDFUNDING TERBUKA</span>
              <h1>Perubahan besar<br />dimulai dari <em>kepedulian.</em></h1>
              <p>
                Temukan campaign yang berarti, ajukan inisiatif kebaikan,
                dan tumbuh bersama komunitas yang peduli.
              </p>
              <div className="story-note">
                <span className="note-icon">✳</span>
                <span><b>Setiap kebaikan punya cerita.</b><small>Jadilah bagian dari cerita berikutnya.</small></span>
              </div>
            </div>
            <span className="auth-copyright">© 2026 Donasi Bersama · Berbuat baik, bersama.</span>
            <span className="story-orb story-orb-one" />
            <span className="story-orb story-orb-two" />
          </div>

          <div className="auth-panel">
            <div className="auth-card">
              <div className="mobile-brand">
                <span className="brand-mark">d.</span>
                <span>donasi<span className="brand-light">bersama</span></span>
              </div>
              <span className="auth-kicker">{authMode === 'login' ? 'SELAMAT DATANG KEMBALI' : 'MULAI BERBUAT BAIK'}</span>
              <h2>{authMode === 'login' ? 'Masuk ke akunmu' : 'Buat akun baru'}</h2>
              <p className="auth-subtitle">
                {authMode === 'login'
                  ? 'Lanjutkan langkah kecil yang berarti.'
                  : 'Bergabung dan mulai dukung perubahan.'}
              </p>

              <div className="auth-tabs" role="tablist" aria-label="Jenis autentikasi">
                <button
                  type="button"
                  role="tab"
                  aria-selected={authMode === 'login'}
                  className={authMode === 'login' ? 'active' : ''}
                  onClick={() => { setAuthMode('login'); setAuthError(''); }}
                >
                  Masuk
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={authMode === 'register'}
                  className={authMode === 'register' ? 'active' : ''}
                  onClick={() => { setAuthMode('register'); setAuthError(''); }}
                >
                  Daftar
                </button>
              </div>

              <form className="auth-form" onSubmit={(event) => { event.preventDefault(); auth(authMode === 'login' ? 'login' : 'register'); }}>
                <label htmlFor="auth-email">Alamat email</label>
                <input
                  id="auth-email"
                  type="email"
                  placeholder="nama@email.com"
                  autoComplete="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                />
                <div className="password-label">
                  <label htmlFor="auth-password">Kata sandi</label>
                  {authMode === 'register' && <span>Minimal 10 karakter</span>}
                </div>
                <input
                  id="auth-password"
                  type="password"
                  placeholder="Masukkan kata sandi"
                  autoComplete={authMode === 'login' ? 'current-password' : 'new-password'}
                  minLength={authMode === 'register' ? 10 : undefined}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
                {authError && <p className="auth-error" role="alert">{authError}</p>}
                <button className="auth-submit" type="submit" disabled={authLoading}>
                  {authLoading
                    ? 'Mohon tunggu…'
                    : authMode === 'login' ? 'Masuk ke akun' : 'Buat akun Donatur'}
                  {!authLoading && <span aria-hidden="true">→</span>}
                </button>
              </form>

              <p className="auth-footnote">
                Dengan melanjutkan, kamu setuju menggunakan platform ini untuk
                mendukung campaign secara bertanggung jawab.
              </p>
              <div className="auth-security"><span>✓</span> Aman · Transparan · Bersama</div>
            </div>
          </div>
        </section>
      </main>
    );
  }

  return (
    <main>
      <header>
        <div>
          <h1>🤝 Donasi Bersama</h1>
          <p>Platform Crowdfunding · Kelompok E</p>
        </div>
        <span className="status">Gateway: {health}</span>
      </header>

      <section className="stats">
        <article><b>{campaigns.length}</b><span>Campaign publik</span></article>
        <article>
          <b>{userRole === 'ADMIN' ? pendingCampaigns.length : 'Admin'}</b>
          <span>{userRole === 'ADMIN' ? 'Menunggu persetujuan' : 'Peninjau campaign'}</span>
        </article>
        <article><b>Real-time</b><span>Notifikasi WebSocket</span></article>
      </section>

      <div className="layout">
        <section>
          <h2>Daftar Campaign Publik</h2>
          <div className="cards">
            {campaigns.map((campaign) => (
              <article className="campaign" key={campaign.id}>
                <small>CAMPAIGN #{campaign.id}</small>
                <h3>{campaign.title}</h3>
                <p>{campaign.description}</p>
                <b>
                  Rp {Number(campaign.collected).toLocaleString('id-ID')} / Rp{' '}
                  {Number(campaign.target).toLocaleString('id-ID')}
                </b>
                <div className="bar">
                  <i style={{ width: `${Math.min(100, campaign.collected / campaign.target * 100)}%` }} />
                </div>
                <small>Status: {campaign.status}</small>
              </article>
            ))}
            {!campaigns.length && <p>Belum ada campaign yang disetujui Admin.</p>}
          </div>
        </section>

        <aside>
          <h2>Akun</h2>
          <p>Login sebagai: <b>{userRole || 'pengguna'}</b></p>
          <button onClick={logout}>Keluar</button>
          <h2>Aktivitas real-time</h2>
          {events.map((event, index) => (
            <p className="event" key={index}>
              {event.event || 'Aktivitas'} · {event.campaign?.title || event.campaign_id || ''}
            </p>
          ))}
          {!events.length && <small>Menunggu aktivitas dari Notification Service...</small>}
        </aside>
      </div>

      {token && (
        <section className="create">
          <h2>Ajukan Campaign</h2>
          <form onSubmit={createCampaign}>
            <input
              placeholder="Judul campaign"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              required
              minLength="3"
            />
            <input
              type="number"
              placeholder="Target donasi (Rupiah)"
              value={target}
              onChange={(event) => setTarget(event.target.value)}
              required
              min="1"
            />
            <button>Kirim untuk ditinjau Admin</button>
          </form>
          <small>Campaign baru tidak tampil untuk publik sebelum disetujui Admin.</small>
        </section>
      )}

      {token && userRole === 'ADMIN' && (
        <section className="create">
          <h2>Persetujuan Campaign</h2>
          <div className="cards">
            {pendingCampaigns.map((campaign) => (
              <article className="campaign" key={campaign.id}>
                <small>CAMPAIGN #{campaign.id} · MENUNGGU</small>
                <h3>{campaign.title}</h3>
                <p>{campaign.description}</p>
                <b>Target: Rp {Number(campaign.target).toLocaleString('id-ID')}</b>
                <div>
                  <button onClick={() => approveCampaign(campaign.id)}>Setujui dan publikasikan</button>
                </div>
              </article>
            ))}
            {!pendingCampaigns.length && <p>Tidak ada campaign yang menunggu persetujuan.</p>}
          </div>
        </section>
      )}

      <footer>Kelompok E · Implementasi sampai Sesi 10 · Go, React, GraphQL, PostgreSQL</footer>
    </main>
  );
}

createRoot(document.getElementById('root')).render(<App />);
