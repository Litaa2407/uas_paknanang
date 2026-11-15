import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const GATEWAY = 'http://localhost:8000';

function claimsFromToken(token) {
  try {
    const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(payload));
  } catch {
    return {};
  }
}

function roleFromToken(token) {
  return claimsFromToken(token).role || '';
}

function CampaignMedia({ campaign, fallbackIndex = 0, interactive = true }) {
  const media = campaign.media || [];
  if (!media.length) {
    return <div className="campaign-art" aria-hidden="true"><span>{['✳', '⌂', '♡'][fallbackIndex % 3]}</span></div>;
  }
  return (
    <div className={`campaign-media-gallery media-count-${media.length}${interactive ? ' detail-media-gallery' : ''}`}>
      {media.map((item) => {
        const url = `${GATEWAY}/campaigns/${campaign.id}/media/${item.id}`;
        return item.mediaType === 'VIDEO'
          ? <video key={item.id} src={url} controls={interactive} preload="metadata" aria-label={`Video ${campaign.title}`} />
          : interactive
            ? <a className="campaign-media-image" key={item.id} href={url} target="_blank" rel="noreferrer" aria-label={`Lihat foto campaign ${campaign.title}`}>
                <img src={url} alt={campaign.title} loading="lazy" />
              </a>
            : <div className="campaign-media-image" key={item.id}>
                <img src={url} alt={campaign.title} loading="lazy" />
              </div>;
      })}
    </div>
  );
}

function App() {
  const [campaigns, setCampaigns] = useState([]);
  const [campaignsLoading, setCampaignsLoading] = useState(true);
  const [campaignsError, setCampaignsError] = useState('');
  const [donationAmount, setDonationAmount] = useState('10000');
  const [donorName, setDonorName] = useState(() => claimsFromToken(localStorage.getItem('token') || '').name || '');
  const [donorEmail, setDonorEmail] = useState(() => claimsFromToken(localStorage.getItem('token') || '').email || '');
  const [donorPhone, setDonorPhone] = useState('');
  const [donationError, setDonationError] = useState('');
  const [donationMessage, setDonationMessage] = useState('');
  const [donationReference, setDonationReference] = useState('');
  const [donationLoading, setDonationLoading] = useState(false);
  const [pendingCampaigns, setPendingCampaigns] = useState([]);
  const [token, setToken] = useState(localStorage.getItem('token') || '');
  const [userRole, setUserRole] = useState(() => roleFromToken(localStorage.getItem('token') || ''));
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState('login');
  const [authError, setAuthError] = useState('');
  const [authLoading, setAuthLoading] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [target, setTarget] = useState('');
  const [photos, setPhotos] = useState([]);
  const [video, setVideo] = useState(null);
  const [campaignError, setCampaignError] = useState('');
  const [campaignLoading, setCampaignLoading] = useState(false);
  const [events, setEvents] = useState([]);
  const [health, setHealth] = useState('memeriksa');
  const donationPageMatch = window.location.pathname.match(/^\/campaign\/(\d+)\/donate\/?$/);
  const donationCampaignId = donationPageMatch?.[1];
  const isDonationPage = Boolean(donationCampaignId);
  const campaignDetailMatch = window.location.pathname.match(/^\/campaign\/(\d+)\/?$/);
  const detailCampaignId = campaignDetailMatch?.[1];
  const isDetailPage = Boolean(detailCampaignId);
  const isCreatePage = window.location.pathname === '/campaign/new';
  const photoPreviews = useMemo(() => photos.map((photo) => URL.createObjectURL(photo)), [photos]);
  const videoPreview = useMemo(() => video ? URL.createObjectURL(video) : '', [video]);

  useEffect(() => () => photoPreviews.forEach((url) => URL.revokeObjectURL(url)), [photoPreviews]);
  useEffect(() => () => { if (videoPreview) URL.revokeObjectURL(videoPreview); }, [videoPreview]);

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
    try {
      const result = await gql('{ campaigns { id title description target collected status media { id mediaType contentType } } }');
      if (result.errors) {
        setCampaignsError('Campaign belum dapat dimuat. Coba muat ulang halaman.');
        return;
      }
      if (result.data?.campaigns) setCampaigns(result.data.campaigns);
    } catch {
      setCampaignsError('Tidak dapat terhubung untuk memuat campaign. Periksa koneksi lalu coba lagi.');
    } finally {
      setCampaignsLoading(false);
    }
  }

  async function loadPendingCampaigns() {
    const result = await gql('{ pendingCampaigns { id title description target collected status } }');
    if (result.errors) {
      alert(result.errors[0].message);
      return;
    }
    setPendingCampaigns(result.data?.pendingCampaigns || []);
  }

  function selectPhotos(files) {
    const selected = Array.from(files || []);
    if (selected.length > 5) {
      setCampaignError('Pilih maksimal 5 foto.');
      return;
    }
    if (selected.some((file) => !['image/jpeg', 'image/png', 'image/webp'].includes(file.type))) {
      setCampaignError('Format foto harus JPG, PNG, atau WebP.');
      return;
    }
    if (selected.some((file) => file.size > 5 * 1024 * 1024)) {
      setCampaignError('Ukuran setiap foto maksimal 5 MB.');
      return;
    }
    setCampaignError('');
    setPhotos(selected);
  }

  function selectVideo(file) {
    if (!file) {
      setVideo(null);
      return;
    }
    if (file.type !== 'video/mp4') {
      setCampaignError('Format video harus MP4.');
      return;
    }
    if (file.size > 25 * 1024 * 1024) {
      setCampaignError('Ukuran video maksimal 25 MB.');
      return;
    }
    setCampaignError('');
    setVideo(file);
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
      setDonorName(data.user?.name || claimsFromToken(data.token).name || '');
      setDonorEmail(data.user?.email || claimsFromToken(data.token).email || '');
      localStorage.setItem('token', data.token);
    } catch {
      setAuthError('Tidak dapat terhubung ke server. Pastikan aplikasi sedang berjalan.');
    } finally {
      setAuthLoading(false);
    }
  }

  async function createCampaign(event) {
    event.preventDefault();
    if (photos.length === 0) {
      setCampaignError('Pilih minimal 1 foto campaign.');
      return;
    }
    try {
      setCampaignLoading(true);
      setCampaignError('');
      const formData = new FormData();
      formData.append('title', title);
      formData.append('description', description);
      formData.append('target', target);
      photos.forEach((photo) => formData.append('photos', photo));
      if (video) formData.append('video', video);

      const response = await fetch(`${GATEWAY}/campaigns`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!response.ok) {
        setCampaignError(await response.text() || 'Campaign tidak dapat dikirim. Periksa kembali isian dan file.');
        return;
      }
      alert('Campaign berhasil dikirim dan menunggu persetujuan Admin sebelum ditampilkan publik.');
      window.location.assign('/');
    } catch {
      setCampaignError('Tidak dapat mengirim campaign. Periksa koneksi dan coba lagi.');
    } finally {
      setCampaignLoading(false);
    }
  }

  async function simulateDonation(event, campaignId) {
    event.preventDefault();
    const amount = Number(donationAmount);
    if (!Number.isSafeInteger(amount) || amount < 1000) {
      setDonationError('Masukkan nominal minimal Rp 1.000.');
      setDonationMessage('');
      return;
    }

    try {
      setDonationLoading(true);
      setDonationError('');
      setDonationMessage('');
      const response = await fetch(`${GATEWAY}/donations/simulate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          campaign_id: Number(campaignId),
          amount,
          donor_name: donorName,
          donor_email: donorEmail,
          donor_phone: donorPhone,
        }),
      });
      if (!response.ok) {
        setDonationError(await response.text() || 'Donasi simulasi gagal diproses.');
        return;
      }
      const result = await response.json();
      setDonationReference(result.reference || '');
      setDonationMessage(`Dukungan simulasi sebesar Rp ${amount.toLocaleString('id-ID')} berhasil dicatat.`);
      await loadCampaigns();
    } catch {
      setDonationError('Tidak dapat terhubung untuk mencatat donasi. Coba lagi.');
    } finally {
      setDonationLoading(false);
    }
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

  if (isDonationPage) {
    const campaign = campaigns.find((item) => String(item.id) === donationCampaignId);

    return (
      <main className="dashboard donation-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <span className="account-avatar">{(userRole || 'P').slice(0, 1)}</span>
            <span className="account-role">{userRole || 'Pengguna'}</span>
            <button className="logout-button" onClick={logout}>Keluar</button>
          </div>
        </header>

        <section className="donation-checkout">
          <a className="back-link" href={`/campaign/${donationCampaignId}`}>← Kembali ke campaign</a>
          {campaignsLoading && <div className="detail-message">Memuat informasi campaign...</div>}
          {!campaignsLoading && campaignsError && <div className="detail-message" role="alert">{campaignsError}</div>}
          {!campaignsLoading && !campaignsError && !campaign && (
            <div className="detail-message">
              <span className="section-kicker">CAMPAIGN TIDAK DITEMUKAN</span>
              <h1>Campaign ini tidak tersedia</h1>
              <p>Kembali ke daftar campaign untuk memilih campaign yang aktif.</p>
            </div>
          )}
          {campaign && userRole !== 'DONATUR' && (
            <div className="detail-message">
              <span className="section-kicker">KHUSUS DONATUR</span>
              <h1>Masuk dengan akun Donatur</h1>
              <p>Hanya akun Donatur yang dapat mengirim dukungan simulasi.</p>
            </div>
          )}
          {campaign && userRole === 'DONATUR' && (
            <>
              <div className="donation-page-heading">
                <span className="section-kicker">DUKUNG CAMPAIGN</span>
                <h1>Lengkapi dukunganmu</h1>
                <p>Isi informasi donatur dan pilih nominal untuk melanjutkan.</p>
              </div>
              <div className="donation-checkout-layout">
                <aside className="donation-campaign-summary">
                  <CampaignMedia campaign={campaign} interactive={false} />
                  <div className="donation-summary-body">
                    <span className="section-kicker">CAMPAIGN PILIHANMU</span>
                    <h2>{campaign.title}</h2>
                    <p>{campaign.description || 'Setiap dukungan kecil dapat membawa perubahan besar bagi mereka yang membutuhkan.'}</p>
                    <div className="donation-summary-progress">
                      <span>Progres saat ini</span>
                      <b>Rp {Number(campaign.collected).toLocaleString('id-ID')}</b>
                    </div>
                    <div className="bar"><i style={{ width: `${Math.min(100, Math.max(0, Number(campaign.collected) / Number(campaign.target) * 100))}%` }} /></div>
                    <small>Target Rp {Number(campaign.target).toLocaleString('id-ID')}</small>
                  </div>
                </aside>

                <section className="donation-checkout-card">
                  {donationMessage ? (
                    <div className="donation-success" role="status">
                      <span className="donation-success-icon">✓</span>
                      <span className="section-kicker">DUKUNGAN TERCATAT</span>
                      <h2>Terima kasih sudah peduli!</h2>
                      <p>{donationMessage}</p>
                      {donationReference && <small>Referensi simulasi: {donationReference}</small>}
                      <a className="donation-submit donation-return" href={`/campaign/${campaign.id}`}>Kembali ke campaign <span aria-hidden="true">→</span></a>
                    </div>
                  ) : (
                    <form className="donation-form donation-checkout-form" onSubmit={(event) => simulateDonation(event, campaign.id)}>
                      <div className="checkout-section-heading">
                        <span className="form-step">01</span>
                        <div><h2>Nominal dukungan</h2><p>Pilih nominal atau masukkan sendiri.</p></div>
                      </div>
                      <div className="donation-presets" aria-label="Pilih nominal donasi">
                        {[10000, 25000, 50000].map((amount) => (
                          <button
                            className={donationAmount === String(amount) ? 'selected' : ''}
                            key={amount}
                            type="button"
                            onClick={() => { setDonationAmount(String(amount)); setDonationError(''); }}
                          >
                            Rp {amount.toLocaleString('id-ID')}
                          </button>
                        ))}
                      </div>
                      <div className="donation-input">
                        <span>Rp</span>
                        <input
                          id="donation-amount"
                          type="number"
                          min="1000"
                          step="1000"
                          value={donationAmount}
                          onChange={(event) => { setDonationAmount(event.target.value); setDonationError(''); }}
                          required
                        />
                      </div>

                      <div className="checkout-section-heading donor-info-heading">
                        <span className="form-step">02</span>
                        <div><h2>Informasi donatur</h2><p>Detail ini dicatat bersama donasi simulasi.</p></div>
                      </div>
                      <div className="donor-fields">
                        <label htmlFor="donor-name">Nama lengkap <span>*</span></label>
                        <input
                          id="donor-name"
                          type="text"
                          autoComplete="name"
                          maxLength="100"
                          value={donorName}
                          onChange={(event) => setDonorName(event.target.value)}
                          required
                        />
                        <label htmlFor="donor-phone">Nomor telepon <span>*</span></label>
                        <input
                          id="donor-phone"
                          type="tel"
                          autoComplete="tel"
                          inputMode="tel"
                          maxLength="30"
                          pattern="[+0-9() -]{8,30}"
                          placeholder="+62 812-3456-7890"
                          value={donorPhone}
                          onChange={(event) => setDonorPhone(event.target.value)}
                          required
                        />
                        <label htmlFor="donor-email">Email <span>*</span></label>
                        <input
                          id="donor-email"
                          type="email"
                          autoComplete="email"
                          maxLength="150"
                          value={donorEmail}
                          onChange={(event) => setDonorEmail(event.target.value)}
                          required
                        />
                      </div>

                      <p className="donation-privacy-note">Informasi ini disimpan bersama catatan donasi dan tidak ditampilkan di aktivitas publik. Ini hanya simulasi, bukan pembayaran uang sungguhan.</p>
                      {donationError && <p className="donation-feedback error" role="alert">{donationError}</p>}
                      <button className="donation-submit" type="submit" disabled={donationLoading}>
                        {donationLoading ? 'Mencatat dukungan...' : 'Konfirmasi dukungan simulasi'}
                        {!donationLoading && <span aria-hidden="true">→</span>}
                      </button>
                    </form>
                  )}
                </section>
              </div>
            </>
          )}
        </section>
        <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Berbuat baik, bersama. · © 2026</span></footer>
      </main>
    );
  }

  if (isDetailPage) {
    const campaign = campaigns.find((item) => String(item.id) === detailCampaignId);
    const progress = campaign && campaign.target > 0
      ? Math.min(100, Math.max(0, Number(campaign.collected) / Number(campaign.target) * 100))
      : 0;

    return (
      <main className="dashboard campaign-detail-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <span className="account-avatar">{(userRole || 'P').slice(0, 1)}</span>
            <span className="account-role">{userRole || 'Pengguna'}</span>
            <button className="logout-button" onClick={logout}>Keluar</button>
          </div>
        </header>

        <section className="campaign-detail">
          <a className="back-link" href="/">← Kembali ke campaign</a>
          {campaignsLoading && <div className="detail-message">Memuat detail campaign...</div>}
          {!campaignsLoading && campaignsError && <div className="detail-message" role="alert">{campaignsError}</div>}
          {!campaignsLoading && !campaignsError && !campaign && (
            <div className="detail-message">
              <span className="section-kicker">CAMPAIGN TIDAK DITEMUKAN</span>
              <h1>Halaman campaign tidak tersedia</h1>
              <p>Campaign ini mungkin belum disetujui atau sudah tidak tersedia.</p>
            </div>
          )}
          {campaign && (
            <>
              <div className="campaign-detail-heading">
                <span className="campaign-status">{campaign.status}</span>
                <span className="section-kicker">CAMPAIGN #{campaign.id}</span>
                <h1>{campaign.title}</h1>
                <p>Dukung cerita dan inisiatif baik ini bersama-sama.</p>
              </div>
              <CampaignMedia campaign={campaign} interactive />
              <div className="campaign-detail-layout">
                <article className="campaign-story">
                  <span className="section-kicker">CERITA CAMPAIGN</span>
                  <h2>Mengapa dukunganmu berarti</h2>
                  <p>{campaign.description || 'Setiap dukungan kecil dapat membawa perubahan besar bagi mereka yang membutuhkan.'}</p>
                </article>
                <aside className="campaign-detail-progress">
                  <span className="section-kicker">PROGRES PENGGALANGAN</span>
                  <strong>Rp {Number(campaign.collected).toLocaleString('id-ID')}</strong>
                  <span className="detail-target">terkumpul (simulasi) dari Rp {Number(campaign.target).toLocaleString('id-ID')}</span>
                  <div className="bar" role="progressbar" aria-label={`Dana terkumpul ${campaign.title}`} aria-valuenow={Math.round(progress)} aria-valuemin="0" aria-valuemax="100">
                    <i style={{ width: `${progress}%` }} />
                  </div>
                  <b className="detail-progress-percent">{progress.toLocaleString('id-ID', { maximumFractionDigits: 0 })}% tercapai</b>
                  <div className="detail-progress-note"><span>♡</span><p>Terima kasih sudah peduli. Dukungan bersama membuat perubahan menjadi mungkin.</p></div>
                  <p className="demo-warning">Mode demo: donasi ini hanya simulasi dan tidak melibatkan uang sungguhan.</p>
                  {userRole === 'DONATUR'
                    ? <a className="donation-submit donation-cta" href={`/campaign/${campaign.id}/donate`}>Lanjutkan untuk berdonasi <span aria-hidden="true">→</span></a>
                    : <p className="donation-feedback">Masuk dengan akun Donatur untuk mengirim dukungan simulasi.</p>}
                </aside>
              </div>
            </>
          )}
        </section>
        <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Berbuat baik, bersama. · © 2026</span></footer>
      </main>
    );
  }

  if (isCreatePage) {
    return (
      <main className="dashboard create-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <span className="account-avatar">{(userRole || 'P').slice(0, 1)}</span>
            <span className="account-role">{userRole || 'Pengguna'}</span>
            <button className="logout-button" onClick={logout}>Keluar</button>
          </div>
        </header>

        <section className="create-page-heading">
          <a className="back-link" href="/">← Kembali ke beranda</a>
          <div className="create-page-title">
            <div>
              <span className="section-kicker">PUNYA INISIATIF BAIK?</span>
              <h1>Mulai campaign-mu</h1>
              <p>Ide baik selalu layak diperjuangkan. Ceritakan rencanamu dan ajak lebih banyak orang ikut peduli.</p>
            </div>
            <span className="create-title-art" aria-hidden="true">✳</span>
          </div>
        </section>

        <section className="create create-page-card">
          <form onSubmit={createCampaign}>
            <div className="form-section-heading">
              <span className="form-step">01</span>
              <div><h2>Informasi campaign</h2><p>Mulai dengan gambaran utama campaign-mu.</p></div>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-title">Nama campaign <span>*</span></label>
              <input
                id="campaign-title"
                placeholder="Contoh: Bantu renovasi perpustakaan desa"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                required
                minLength="3"
                maxLength="100"
              />
              <small>Gunakan judul yang jelas dan mudah dipahami.</small>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-description">Cerita campaign</label>
              <textarea
                id="campaign-description"
                placeholder="Ceritakan siapa yang ingin kamu bantu, apa kebutuhannya, dan bagaimana dana akan digunakan..."
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                rows="5"
              />
              <small>Deskripsi membantu calon pendukung memahami tujuan campaign.</small>
            </div>
            <div className="form-section-heading target-heading">
              <span className="form-step">02</span>
              <div><h2>Tambahkan media</h2><p>Foto dan video membantu orang memahami campaign-mu.</p></div>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-photos">Foto campaign <span>* Wajib, minimal 1 foto</span></label>
              <label className="upload-dropzone" htmlFor="campaign-photos">
                <span className="upload-icon">↑</span>
                <span><b>Pilih foto campaign</b><small>JPG, PNG, WebP · Wajib min. 1, maks. 5 foto · 5 MB per foto</small></span>
                <input id="campaign-photos" type="file" accept="image/jpeg,image/png,image/webp" multiple required aria-required="true" onChange={(event) => selectPhotos(event.target.files)} />
              </label>
              {photos.length > 0 && (
                <div className="selected-media-list">
                  {photos.map((photo, index) => (
                    <div className="selected-media-item" key={`${photo.name}-${photo.lastModified}`}>
                      <img src={photoPreviews[index]} alt="" />
                      <span title={photo.name}>{photo.name}</span>
                      <button type="button" onClick={() => setPhotos((current) => current.filter((_, itemIndex) => itemIndex !== index))} aria-label={`Hapus ${photo.name}`}>×</button>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className="field-group">
              <label htmlFor="campaign-video">Video campaign <span className="optional-label">Opsional · 1 video</span></label>
              <label className="upload-dropzone video-dropzone" htmlFor="campaign-video">
                <span className="upload-icon">▶</span>
                <span><b>{video ? video.name : 'Pilih video'}</b><small>MP4 · Maks. 25 MB</small></span>
                <input id="campaign-video" type="file" accept="video/mp4" onChange={(event) => selectVideo(event.target.files?.[0])} />
              </label>
              {video && <button className="remove-video-button" type="button" onClick={() => setVideo(null)}>Hapus video</button>}
            </div>
            <div className="form-section-heading target-heading">
              <span className="form-step">03</span>
              <div><h2>Tentukan target</h2><p>Berapa dana yang dibutuhkan?</p></div>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-target">Target donasi <span>*</span></label>
              <div className="currency-input"><span>Rp</span><input
                id="campaign-target"
                type="number"
                placeholder="10.000.000"
                value={target}
                onChange={(event) => setTarget(event.target.value)}
                required
                min="1"
              /></div>
              <small>Tentukan target realistis untuk kebutuhan campaign.</small>
            </div>
            {campaignError && <p className="campaign-form-error" role="alert">{campaignError}</p>}
            <div className="form-actions">
              <a href="/">Batal</a>
              <button disabled={campaignLoading}>
                {campaignLoading ? 'Mengirim...' : 'Kirim campaign'}
                {!campaignLoading && <span aria-hidden="true">→</span>}
              </button>
            </div>
          </form>
          <aside className="create-sidebar">
            <div className="campaign-preview">
              <div className="preview-art">
                {videoPreview
                  ? <video src={videoPreview} controls preload="metadata" aria-label="Pratinjau video campaign" />
                  : photoPreviews[0] ? <img src={photoPreviews[0]} alt="Pratinjau foto campaign" />
                    : <span>✳</span>}
                <small>PRATINJAU CAMPAIGN</small>
              </div>
              {photoPreviews.length > 1 && <div className="preview-thumbnails">{photoPreviews.slice(1).map((url, index) => <img src={url} alt={`Foto campaign ${index + 2}`} key={url} />)}</div>}
              <div className="preview-body">
                <span className="preview-status"><i /> Menunggu persetujuan</span>
                <h3>{title || 'Judul campaign-mu'}</h3>
                <p>{description || 'Cerita dan tujuan campaign akan tampil di sini agar mudah dipahami calon pendukung.'}</p>
                <div className="preview-rule" />
                <small>Target donasi</small>
                <b>{target ? `Rp ${Number(target).toLocaleString('id-ID')}` : 'Rp 0'}</b>
              </div>
            </div>
            <div className="review-note"><span>✓</span><p><b>Aman dan transparan</b>Setiap campaign ditinjau Admin sebelum dipublikasikan.</p></div>
            <div className="create-help"><span>♡</span><p><b>Tips campaign yang baik</b><br />Ceritakan kebutuhan dengan jujur, jelas, dan spesifik.</p></div>
          </aside>
        </section>
      </main>
    );
  }

  return (
    <main className="dashboard">
      <header className="dashboard-header">
        <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
          <span className="brand-mark">d.</span>
          <span>donasi<span className="brand-light">bersama</span></span>
        </a>
        <div className="header-account">
          <span className={`status ${health}`}><i /> {health === 'online' ? 'Semua sistem aktif' : health === 'offline' ? 'Gateway offline' : 'Menghubungkan...'}</span>
          <span className="account-avatar">{(userRole || 'P').slice(0, 1)}</span>
          <span className="account-role">{userRole || 'Pengguna'}</span>
          <a className="create-campaign-link" href="/campaign/new">Ajukan campaign <span aria-hidden="true">+</span></a>
          <button className="logout-button" onClick={logout}>Keluar</button>
        </div>
      </header>

      <section className="dashboard-hero">
        <div className="hero-copy">
          <span className="hero-kicker"><i /> MULAI DARI CERITA MEREKA</span>
          <h1>Temukan cerita,<br />mulai dari <em>kepedulian.</em></h1>
          <p>Jelajahi inisiatif komunitas dan kenali cerita di balik setiap campaign.</p>
          <a className="hero-link" href="#campaigns">Jelajahi campaign <span aria-hidden="true">↓</span></a>
        </div>
        <div className="hero-art" aria-hidden="true">
          <div className="hero-sun" />
          <div className="hero-leaf leaf-one" />
          <div className="hero-leaf leaf-two" />
          <div className="hero-leaf leaf-three" />
          <div className="hero-art-caption"><span>01</span> Kebaikan yang tumbuh bersama</div>
        </div>
      </section>

      <div className="dashboard-content">
        <section className="campaign-section" id="campaigns">
          <div className="section-heading">
            <div><span className="section-kicker">CERITA YANG MENGGERAKKAN</span><h2>Temukan campaign</h2></div>
          </div>
          <div className="cards">
            {campaigns.map((campaign, index) => {
              const progress = campaign.target > 0
                ? Math.min(100, Math.max(0, Number(campaign.collected) / Number(campaign.target) * 100))
                : 0;
              return (
                <article className={`campaign campaign-${index % 3} campaign-clickable`} key={campaign.id}>
                  <CampaignMedia campaign={campaign} fallbackIndex={index} interactive={false} />
                  <div className="campaign-body">
                    <div className="campaign-meta"><small>CAMPAIGN #{campaign.id}</small><span className="campaign-status">{campaign.status}</span></div>
                    <h3>{campaign.title}</h3>
                    <p>{campaign.description || 'Setiap dukungan kecil dapat membawa perubahan besar bagi mereka yang membutuhkan.'}</p>
                    <div className="campaign-progress-label"><span>Terkumpul (simulasi)</span><b>{progress.toLocaleString('id-ID', { maximumFractionDigits: 0 })}%</b></div>
                    <div className="bar" role="progressbar" aria-label={`Dana terkumpul ${campaign.title}`} aria-valuenow={Math.round(progress)} aria-valuemin="0" aria-valuemax="100">
                      <i style={{ width: `${progress}%` }} />
                    </div>
                    <div className="campaign-amount">
                      <b>Rp {Number(campaign.collected).toLocaleString('id-ID')}</b>
                      <span>dari Rp {Number(campaign.target).toLocaleString('id-ID')}</span>
                    </div>
                  </div>
                  <a className="campaign-card-link" href={`/campaign/${campaign.id}`} aria-label={`Lihat detail campaign ${campaign.title}`} />
                </article>
              );
            })}
            {!campaignsLoading && campaignsError && <div className="empty-state" role="alert"><span>!</span><h3>Campaign gagal dimuat</h3><p>{campaignsError}</p></div>}
            {!campaigns.length && (
              <div className="empty-state">
                <span>✳</span><h3>Campaign sedang disiapkan</h3>
                <p>Belum ada campaign yang dipublikasikan. Silakan kembali lagi nanti.</p>
              </div>
            )}
          </div>
        </section>

        <aside className="activity-panel">
          <div className="section-heading">
            <div><span className="section-kicker">KABAR TERBARU</span><h2>Aktivitas</h2></div>
            <span className="live-indicator"><i /> LIVE</span>
          </div>
          <div className="activity-list">
            {events.map((event, index) => (
              <div className="activity-item" key={index}>
                <span className="activity-icon">↗</span>
                <p><b>{event.event === 'SIMULATED_DONATION' ? 'Donasi simulasi' : event.event || 'Aktivitas baru'}</b><span>{event.campaign?.title || (event.campaign_id ? `Campaign #${event.campaign_id}` : 'Aktivitas campaign')}</span></p>
              </div>
            ))}
            {!events.length && <div className="activity-empty"><span>✳</span><p>Belum ada aktivitas baru.<small>Aktivitas campaign akan muncul di sini.</small></p></div>}
          </div>
          <div className="activity-note"><span>♡</span><p>Setiap langkah kecil<br /><b>membawa arti besar.</b></p></div>
        </aside>
      </div>

      {token && userRole === 'ADMIN' && (
        <section className="create approval-section">
          <div className="section-heading">
            <div><span className="section-kicker">AREA ADMIN</span><h2>Persetujuan campaign</h2></div>
            <span className="section-count">{pendingCampaigns.length} menunggu</span>
          </div>
          <div className="cards">
            {pendingCampaigns.map((campaign) => (
              <article className="campaign pending-campaign" key={campaign.id}>
                <div className="campaign-body">
                  <div className="campaign-meta"><small>CAMPAIGN #{campaign.id}</small><span className="campaign-status pending">Menunggu</span></div>
                  <h3>{campaign.title}</h3>
                  <p>{campaign.description || 'Belum ada deskripsi campaign.'}</p>
                  <div className="campaign-amount"><b>Rp {Number(campaign.target).toLocaleString('id-ID')}</b><span>Target donasi</span></div>
                  <button onClick={() => approveCampaign(campaign.id)}>Setujui dan publikasikan <span aria-hidden="true">→</span></button>
                </div>
              </article>
            ))}
            {!pendingCampaigns.length && <div className="empty-state"><span>✓</span><h3>Semua sudah ditinjau</h3><p>Tidak ada campaign yang menunggu persetujuan.</p></div>}
          </div>
        </section>
      )}

      <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Berbuat baik, bersama. · © 2026</span></footer>
    </main>
  );
}

createRoot(document.getElementById('root')).render(<App />);
