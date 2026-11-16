import React, { useCallback, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const GATEWAY = import.meta.env.VITE_GATEWAY_URL || '';
const HOME_CAMPAIGNS_COUNT = 3;
const CAMPAIGNS_PER_PAGE = 9;
const PENDING_CAMPAIGNS_PER_PAGE = 10;

function consumeOAuthRedirect() {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ''));
  const token = params.get('oauth_token');
  const error = params.get('oauth_error');
  if (token) localStorage.setItem('token', token);
  if (token || error) {
    window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`);
  }
  return error ? 'Login dengan Google gagal. Silakan coba lagi atau gunakan email.' : '';
}

const oauthRedirectError = consumeOAuthRedirect();

function claimsFromToken(token) {
  try {
    const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(payload));
  } catch {
    return {};
  }
}

function expirationFromToken(token) {
  const expiration = Number(claimsFromToken(token).exp);
  return Number.isFinite(expiration) && expiration > 0 ? expiration * 1000 : 0;
}

function validateCampaignMedia(photos, video) {
  if (!photos.length) return 'Pilih minimal 1 foto campaign.';
  if (photos.length > 5) return 'Maksimal 5 foto dapat diunggah.';
  if (photos.some((file) => file.size > 5 * 1024 * 1024)) return 'Ukuran setiap foto maksimal 5 MB.';
  if (photos.some((file) => file.type && !['image/jpeg', 'image/png', 'image/webp'].includes(file.type))) {
    return 'Format foto harus JPG, PNG, atau WebP.';
  }
  if (video && video.size > 25 * 1024 * 1024) return 'Ukuran video maksimal 25 MB.';
  if (video && video.type && video.type !== 'video/mp4') return 'Format video harus MP4.';
  return '';
}

function formatDateTime(value) {
  if (!value) return 'Waktu pengajuan tidak tersedia';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('id-ID', {
    dateStyle: 'long',
    timeStyle: 'short',
  }).format(date);
}

function storedToken() {
  const savedToken = localStorage.getItem('token') || '';
  if (savedToken && expirationFromToken(savedToken) <= Date.now()) {
    localStorage.removeItem('token');
    return '';
  }
  return savedToken;
}

function roleFromToken(token) {
  return claimsFromToken(token).role || '';
}

function CampaignMediaItem({ campaign, item, interactive, token, onMediaDimensions, onImageClick }) {
  const [mediaURL, setMediaURL] = useState('');
  const [mediaError, setMediaError] = useState(false);
  const sourceURL = `${GATEWAY}/campaigns/${campaign.id}/media/${item.id}`;
  const needsAuth = Boolean(token && campaign.status !== 'AKTIF');

  useEffect(() => {
    setMediaError(false);
    if (!needsAuth) {
      setMediaURL(sourceURL);
      return undefined;
    }

    let active = true;
    let objectURL = '';
    fetch(sourceURL, { headers: { Authorization: `Bearer ${token}` } })
      .then((response) => {
        if (!response.ok) throw new Error('Media campaign tidak dapat dimuat');
        return response.blob();
      })
      .then((blob) => {
        objectURL = URL.createObjectURL(blob);
        if (active) setMediaURL(objectURL);
        else URL.revokeObjectURL(objectURL);
      })
      .catch(() => {
        if (active) {
          setMediaURL('');
          setMediaError(true);
        }
      });

    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [needsAuth, sourceURL, token]);

  if (mediaError) {
    return <div className="campaign-media-error" role="img" aria-label={`Media ${campaign.title} gagal dimuat`}>Media tidak dapat dimuat</div>;
  }
  if (!mediaURL) {
    return <div className="campaign-media-image media-loading" aria-label="Memuat media campaign" />;
  }
  return item.mediaType === 'VIDEO'
    ? <video
        src={mediaURL}
        controls={interactive}
        preload="metadata"
        aria-label={`Video ${campaign.title}`}
        onLoadedMetadata={(event) => onMediaDimensions?.(event.currentTarget.videoWidth / event.currentTarget.videoHeight)}
      />
    : interactive && onImageClick
      ? <button className="campaign-media-image" type="button" onClick={() => onImageClick(mediaURL)} aria-label={`Buka foto ${campaign.title} ukuran penuh`}>
          <img
            src={mediaURL}
            alt={campaign.title}
            loading="lazy"
            onLoad={(event) => onMediaDimensions?.(event.currentTarget.naturalWidth / event.currentTarget.naturalHeight)}
          />
        </button>
      : interactive
      ? <a className="campaign-media-image" href={mediaURL} target="_blank" rel="noreferrer" aria-label={`Lihat foto campaign ${campaign.title}`}>
          <img
            src={mediaURL}
            alt={campaign.title}
            loading="lazy"
            onLoad={(event) => onMediaDimensions?.(event.currentTarget.naturalWidth / event.currentTarget.naturalHeight)}
          />
        </a>
      : <div className="campaign-media-image">
          <img
            src={mediaURL}
            alt={campaign.title}
            loading="lazy"
            onLoad={(event) => onMediaDimensions?.(event.currentTarget.naturalWidth / event.currentTarget.naturalHeight)}
          />
        </div>;
}

function CampaignMedia({ campaign, fallbackIndex = 0, interactive = true, token = '', review = false }) {
  const media = campaign.media || [];
  const [activeMediaIndex, setActiveMediaIndex] = useState(0);
  const [activeMediaAspectRatio, setActiveMediaAspectRatio] = useState(null);
  const [fullSizeImageURL, setFullSizeImageURL] = useState('');
  const [carouselPaused, setCarouselPaused] = useState(false);
  const cardCarousel = !review && !interactive && media.length > 1;
  const autoSlideMediaCount = media.filter((item) => item.mediaType !== 'VIDEO').length;
  useEffect(() => {
    setActiveMediaIndex(Math.max(0, media.findIndex((item) => item.mediaType !== 'VIDEO')));
    setActiveMediaAspectRatio(null);
    setFullSizeImageURL('');
    setCarouselPaused(false);
  }, [campaign.id]);
  useEffect(() => {
    if (!fullSizeImageURL) return undefined;
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setFullSizeImageURL('');
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [fullSizeImageURL]);
  useEffect(() => {
    if (!cardCarousel || carouselPaused || autoSlideMediaCount < 2) return undefined;
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reducedMotion) return undefined;
    const interval = window.setInterval(() => {
      setActiveMediaIndex((current) => {
        const imageIndexes = media.reduce((indexes, item, index) => {
          if (item.mediaType !== 'VIDEO') indexes.push(index);
          return indexes;
        }, []);
        const currentPosition = imageIndexes.indexOf(current);
        return imageIndexes[(currentPosition + 1) % imageIndexes.length];
      });
    }, 4500);
    return () => window.clearInterval(interval);
  }, [autoSlideMediaCount, cardCarousel, carouselPaused, media]);

  if (!media.length) {
    return (
      <div className={`campaign-art${review ? ' campaign-art-review' : ''}`} aria-hidden={review ? undefined : 'true'}>
        <span className="campaign-art-symbol">{['✳', '⌂', '♡'][fallbackIndex % 3]}</span>
        {review && (
          <div className="campaign-art-copy">
            <span>MEDIA CAMPAIGN</span>
            <b>Belum ada foto atau video</b>
            <small>Campaign ini belum memiliki media pendukung.</small>
          </div>
        )}
      </div>
    );
  }
  if (review || interactive || media.length > 1) {
    const activeIndex = activeMediaIndex % media.length;
    const activeMedia = media[activeIndex];
    return (
      <div
        className={`campaign-media-carousel${cardCarousel ? ' campaign-card-media-carousel' : ''}`}
        aria-label="Galeri media campaign"
        onMouseEnter={cardCarousel ? () => setCarouselPaused(true) : undefined}
        onMouseLeave={cardCarousel ? () => setCarouselPaused(false) : undefined}
        onFocusCapture={cardCarousel ? () => setCarouselPaused(true) : undefined}
        onBlurCapture={cardCarousel ? () => setCarouselPaused(false) : undefined}
      >
        <div
          className="campaign-media-gallery media-count-1 detail-media-gallery"
          style={{
            aspectRatio: cardCarousel
              ? undefined
              : activeMediaAspectRatio
                ? String(activeMediaAspectRatio)
                : '16 / 9',
          }}
        >
          <CampaignMediaItem
            campaign={campaign}
            item={activeMedia}
            interactive={interactive}
            token={token}
            onMediaDimensions={cardCarousel ? undefined : setActiveMediaAspectRatio}
            onImageClick={cardCarousel ? undefined : setFullSizeImageURL}
          />
        </div>
        {cardCarousel ? (
          <div className="campaign-card-media-pagination" aria-label={`Media ${activeIndex + 1} dari ${media.length}`}>
            {media.map((item, index) => (
              <span className={index === activeIndex ? 'active' : ''} key={item.id} />
            ))}
          </div>
        ) : (
          <div className="campaign-media-carousel-footer">
          <span className="campaign-media-carousel-count">{activeIndex + 1} / {media.length}</span>
          <div className="campaign-media-carousel-footer-right">
            {activeMedia.mediaType !== 'VIDEO' && <span className="campaign-media-carousel-hint">Klik foto untuk ukuran penuh</span>}
            {media.length > 1 && (
              <>
                <button
                  className="campaign-media-carousel-arrow"
                  type="button"
                  onClick={() => {
                    setActiveMediaIndex((activeIndex + media.length - 1) % media.length);
                    setActiveMediaAspectRatio(null);
                  }}
                  aria-label="Lihat media sebelumnya"
                >
                  ‹
                </button>
                <div className="campaign-media-carousel-dots" aria-label="Pilih media">
                  {media.map((item, index) => (
                    <button
                      key={item.id}
                      type="button"
                      className={index === activeIndex ? 'active' : ''}
                      onClick={() => {
                        setActiveMediaIndex(index);
                        setActiveMediaAspectRatio(null);
                      }}
                      aria-label={`Lihat media ${index + 1}`}
                      aria-current={index === activeIndex ? 'true' : undefined}
                    />
                  ))}
                </div>
                <button
                  className="campaign-media-carousel-arrow"
                  type="button"
                  onClick={() => {
                    setActiveMediaIndex((activeIndex + 1) % media.length);
                    setActiveMediaAspectRatio(null);
                  }}
                  aria-label="Lihat media berikutnya"
                >
                  ›
                </button>
              </>
            )}
          </div>
          </div>
        )}
        {!cardCarousel && fullSizeImageURL && (
          <div className="campaign-media-lightbox" role="dialog" aria-modal="true" aria-label="Foto campaign ukuran penuh" onClick={() => setFullSizeImageURL('')}>
            <button className="campaign-media-lightbox-back" type="button" onClick={() => setFullSizeImageURL('')}>
              <span aria-hidden="true">←</span> Kembali
            </button>
            <img src={fullSizeImageURL} alt={campaign.title} onClick={(event) => event.stopPropagation()} />
          </div>
        )}
      </div>
    );
  }
  return (
    <div className={`campaign-media-gallery media-count-${media.length}${interactive ? ' detail-media-gallery' : ''}`}>
      {media.map((item) => <CampaignMediaItem key={item.id} campaign={campaign} item={item} interactive={interactive} token={token} />)}
    </div>
  );
}

function serviceCounts(service) {
  if (!service || typeof service !== 'object') return null;
  const states = Object.values(service).filter((value) => typeof value === 'boolean');
  if (!states.length) return null;
  return { healthy: states.filter(Boolean).length, total: states.length };
}

function AppDialog({ dialog, onClose }) {
  const cancelButton = React.useRef(null);

  useEffect(() => {
    if (!dialog) return undefined;
    cancelButton.current?.focus();
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [dialog, onClose]);

  if (!dialog) return null;

  return (
    <div className="app-dialog-backdrop" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }}>
      <section
        className={`app-dialog${dialog.kind === 'success' ? ' success' : ''}`}
        role={dialog.confirm ? 'alertdialog' : 'dialog'}
        aria-modal="true"
        aria-labelledby="app-dialog-title"
        aria-describedby="app-dialog-message"
      >
        <span className="app-dialog-icon" aria-hidden="true">{dialog.kind === 'success' ? '✓' : dialog.confirm ? '?' : '!'}</span>
        <h2 id="app-dialog-title">{dialog.title}</h2>
        <p id="app-dialog-message">{dialog.message}</p>
        <div className="app-dialog-actions">
          {dialog.confirm && (
            <button type="button" className="app-dialog-cancel" ref={cancelButton} onClick={onClose}>
              Batal
            </button>
          )}
          <button
            type="button"
            className="app-dialog-accept"
            autoFocus={!dialog.confirm}
            onClick={() => {
              const action = dialog.confirm;
              onClose();
              action?.();
            }}
          >
            {dialog.confirm ? dialog.confirmLabel || 'Ya, setujui' : 'Mengerti'}
          </button>
        </div>
      </section>
    </div>
  );
}

function AccountIdentity({ name, email, role }) {
  const displayName = name || email?.split('@')[0] || 'Pengguna';
  const initials = displayName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toLocaleUpperCase('id-ID'))
    .join('') || 'P';

  return (
    <div className="account-identity" aria-label={`Akun ${displayName}, ${role || 'Pengguna'}`}>
      <span className="account-avatar" aria-hidden="true">{initials}</span>
      <span className="account-identity-copy">
        <b title={displayName}>{displayName}</b>
        <small title={email || role || 'Pengguna'}>{email || role || 'Pengguna'}</small>
      </span>
      <span className="account-role">{role || 'Pengguna'}</span>
    </div>
  );
}

function CampaignCard({ campaign, index, completed = false }) {
  const progress = campaign.target > 0
    ? Math.min(100, Math.max(0, Number(campaign.collected) / Number(campaign.target) * 100))
    : 0;

  return (
    <article className={`campaign campaign-${index % 3} campaign-clickable${completed ? ' campaign-completed' : ''}`} key={campaign.id}>
      <CampaignMedia campaign={campaign} fallbackIndex={index} interactive={false} />
      <div className="campaign-body">
        <div className="campaign-meta"><small>CAMPAIGN #{campaign.id}</small><span className="campaign-status">{completed ? 'SELESAI' : campaign.status}</span></div>
        <h3>{campaign.title}</h3>
        <p>{campaign.description || 'Setiap dukungan kecil dapat membawa perubahan besar bagi mereka yang membutuhkan.'}</p>
        <div className="campaign-progress-label"><span>Dana terkumpul</span><b>{progress.toLocaleString('id-ID', { maximumFractionDigits: 0 })}%</b></div>
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
}

function AppContent({ showDialog }) {
  const [campaigns, setCampaigns] = useState([]);
  const [campaignEngagement, setCampaignEngagement] = useState(null);
  const [campaignEngagementLoading, setCampaignEngagementLoading] = useState(false);
  const [campaignEngagementError, setCampaignEngagementError] = useState('');
  const [campaignEngagementRefresh, setCampaignEngagementRefresh] = useState(0);
  const [campaignDetailTab, setCampaignDetailTab] = useState('story');
  const [campaignQuestion, setCampaignQuestion] = useState('');
  const [campaignQuestionError, setCampaignQuestionError] = useState('');
  const [campaignQuestionMessage, setCampaignQuestionMessage] = useState('');
  const [campaignQuestionLoading, setCampaignQuestionLoading] = useState(false);
  const [campaignAnswerDrafts, setCampaignAnswerDrafts] = useState({});
  const [campaignAnswerError, setCampaignAnswerError] = useState('');
  const [campaignAnswerMessage, setCampaignAnswerMessage] = useState('');
  const [campaignAnswerLoading, setCampaignAnswerLoading] = useState(false);
  const [fundUpdateAmount, setFundUpdateAmount] = useState('');
  const [fundUpdatePurpose, setFundUpdatePurpose] = useState('');
  const [fundUpdateDescription, setFundUpdateDescription] = useState('');
  const [fundUpdateEvidence, setFundUpdateEvidence] = useState(null);
  const [fundUpdateError, setFundUpdateError] = useState('');
  const [fundUpdateMessage, setFundUpdateMessage] = useState('');
  const [fundUpdateLoading, setFundUpdateLoading] = useState(false);
  const [campaignSearch, setCampaignSearch] = useState('');
  const [campaignPage, setCampaignPage] = useState(1);
  const [campaignsLoading, setCampaignsLoading] = useState(true);
  const [campaignsError, setCampaignsError] = useState('');
  const [token, setToken] = useState(storedToken);
  const [userRole, setUserRole] = useState(() => roleFromToken(storedToken()));
  const [donationAmount, setDonationAmount] = useState('10000');
  const [donorName, setDonorName] = useState(() => claimsFromToken(token).name || '');
  const [donorEmail, setDonorEmail] = useState(() => claimsFromToken(token).email || '');
  const tokenClaims = token ? claimsFromToken(token) : {};
  const accountName = donorName || tokenClaims.name || '';
  const accountEmail = donorEmail || tokenClaims.email || '';
  const [donorPhone, setDonorPhone] = useState('');
  const [donationError, setDonationError] = useState('');
  const [donationMessage, setDonationMessage] = useState('');
  const [donationReference, setDonationReference] = useState(() => new URLSearchParams(window.location.search).get('order_id') || '');
  const [donationPaymentStatus, setDonationPaymentStatus] = useState('IDLE');
  const [donationPaidAmount, setDonationPaidAmount] = useState(0);
  const [donationReferenceCopied, setDonationReferenceCopied] = useState(false);
  const [donationLoading, setDonationLoading] = useState(false);
  const [donationStatusLoading, setDonationStatusLoading] = useState(false);
  const [cancellingDonationReference, setCancellingDonationReference] = useState('');
  const [pendingDonations, setPendingDonations] = useState([]);
  const [pendingDonationsLoading, setPendingDonationsLoading] = useState(false);
  const [pendingDonationsError, setPendingDonationsError] = useState('');
  const [donorDonations, setDonorDonations] = useState([]);
  const [donorDonationsLoading, setDonorDonationsLoading] = useState(false);
  const [donorDonationsError, setDonorDonationsError] = useState('');
  const [resumingDonationReference, setResumingDonationReference] = useState('');
  const [pendingCampaigns, setPendingCampaigns] = useState([]);
  const [pendingCampaignPage, setPendingCampaignPage] = useState(1);
  const [pendingCampaignDetail, setPendingCampaignDetail] = useState(null);
  const [pendingCampaignDetailLoading, setPendingCampaignDetailLoading] = useState(false);
  const [pendingCampaignDetailError, setPendingCampaignDetailError] = useState('');
  const [pendingMediaSelections, setPendingMediaSelections] = useState({});
  const [pendingMediaStatuses, setPendingMediaStatuses] = useState({});
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState('login');
  const [authError, setAuthError] = useState(oauthRedirectError);
  const [authLoading, setAuthLoading] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [target, setTarget] = useState('');
  const [campaignPhotos, setCampaignPhotos] = useState([]);
  const [campaignVideo, setCampaignVideo] = useState(null);
  const [createdCampaignId, setCreatedCampaignId] = useState(null);
  const [campaignError, setCampaignError] = useState('');
  const [campaignLoading, setCampaignLoading] = useState(false);
  const [health, setHealth] = useState({
    gateway: 'checking',
    campaign: null,
    notification: null,
    registryAvailable: false,
  });
  const donationPageMatch = window.location.pathname.match(/^\/campaign\/(\d+)\/donate\/?$/);
  const donationCampaignId = donationPageMatch?.[1];
  const isDonationPage = Boolean(donationCampaignId);
  const campaignDetailMatch = window.location.pathname.match(/^\/campaign\/(\d+)\/?$/);
  const detailCampaignId = campaignDetailMatch?.[1];
  const isDetailPage = Boolean(detailCampaignId);
  const detailCampaign = campaigns.find((item) => String(item.id) === detailCampaignId);
  const adminCampaignMatch = window.location.pathname.match(/^\/admin\/campaign\/(\d+)\/?$/);
  const adminCampaignId = adminCampaignMatch?.[1];
  const isAdminCampaignPage = Boolean(adminCampaignId);
  const isCreatePage = window.location.pathname === '/campaign/new';
  const isCampaignsPage = window.location.pathname === '/semua-campaign';
  const isDonasikuPage = window.location.pathname === '/donasiku';
  const completedCampaigns = campaigns.filter((campaign) => (
    campaign.status === 'SELESAI' ||
    (Number(campaign.target) > 0 && Number(campaign.collected) >= Number(campaign.target))
  ));
  const activeCampaigns = campaigns.filter((campaign) => !completedCampaigns.includes(campaign));
  const normalizedCampaignSearch = campaignSearch.trim().toLocaleLowerCase('id-ID');
  const filteredCampaigns = normalizedCampaignSearch
    ? activeCampaigns.filter((campaign) => `${campaign.title} ${campaign.description || ''} ${campaign.id}`.toLocaleLowerCase('id-ID').includes(normalizedCampaignSearch))
    : activeCampaigns;
  const campaignPageCount = isCampaignsPage ? Math.ceil(filteredCampaigns.length / CAMPAIGNS_PER_PAGE) : 1;
  const currentCampaignPage = Math.min(campaignPage, campaignPageCount || 1);
  const visibleCampaigns = isCampaignsPage
    ? filteredCampaigns.slice(
        (currentCampaignPage - 1) * CAMPAIGNS_PER_PAGE,
        currentCampaignPage * CAMPAIGNS_PER_PAGE,
      )
    : filteredCampaigns.slice(0, HOME_CAMPAIGNS_COUNT);
  const pendingCampaignPageCount = Math.ceil(pendingCampaigns.length / PENDING_CAMPAIGNS_PER_PAGE);
  const currentPendingCampaignPage = Math.min(pendingCampaignPage, pendingCampaignPageCount || 1);
  const visiblePendingCampaigns = pendingCampaigns.slice(
    (currentPendingCampaignPage - 1) * PENDING_CAMPAIGNS_PER_PAGE,
    currentPendingCampaignPage * PENDING_CAMPAIGNS_PER_PAGE,
  );
  const gatewayStatus = health.gateway === 'checking'
    ? 'checking'
    : health.gateway === 'offline'
      ? 'offline'
      : health.registryAvailable && health.campaign?.healthy > 0 && health.notification?.healthy > 0
        ? 'online'
        : 'degraded';

  async function gql(query, variables) {
    const response = await fetch(`${GATEWAY}/graphql`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ query, ...(variables ? { variables } : {}) }),
    });
    return response.json();
  }

  async function refreshDonationStatus(reference = donationReference) {
    if (!reference || !token) return;
    setDonationStatusLoading(true);
    setDonationError('');
    try {
      const response = await fetch(`${GATEWAY}/donations/${encodeURIComponent(reference)}/status`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok) {
        setDonationError((await response.text()) || 'Status pembayaran belum dapat diperiksa. Coba lagi.');
        return;
      }
      const result = await response.json();
      setDonationPaymentStatus(result.status);
      setDonationPaidAmount(Number(result.amount) || 0);
      if (result.status === 'SETTLEMENT') {
        setDonationMessage('Pembayaran berhasil diterima. Terima kasih sudah berdonasi!');
        const previousCampaign = campaigns.find((item) => String(item.id) === String(donationCampaignId));
        const updatedCampaigns = await loadCampaigns();
        const updatedCampaign = updatedCampaigns.find((item) => String(item.id) === String(donationCampaignId));
        if (
          previousCampaign &&
          updatedCampaign &&
          Number(previousCampaign.collected) < Number(updatedCampaign.target) &&
          Number(updatedCampaign.collected) >= Number(updatedCampaign.target)
        ) {
          showDialog({
            kind: 'success',
            title: 'Target campaign tercapai!',
            message: `Yeay! Campaign “${updatedCampaign.title}” sudah mencapai target Rp ${Number(updatedCampaign.target).toLocaleString('id-ID')} berkat dukungan para donatur, termasuk kamu.`,
          });
        }
      } else if (result.status === 'FAILED') {
        setDonationMessage('Pembayaran belum berhasil. Kamu dapat mencoba berdonasi kembali.');
      } else {
        setDonationMessage('Pembayaran masih menunggu konfirmasi. Jika baru saja membayar, tunggu sebentar lalu periksa kembali.');
      }
    } catch {
      setDonationError('Tidak dapat terhubung untuk memeriksa pembayaran. Coba lagi.');
    } finally {
      setDonationStatusLoading(false);
    }
  }

  async function copyDonationReference() {
    try {
      await navigator.clipboard.writeText(donationReference);
      setDonationReferenceCopied(true);
      window.setTimeout(() => setDonationReferenceCopied(false), 1800);
    } catch {
      setDonationError('Referensi pembayaran belum dapat disalin. Silakan salin secara manual.');
    }
  }

  async function loadPendingDonations(campaignId, signal) {
    if (!token || userRole !== 'DONATUR' || !campaignId) {
      setPendingDonations([]);
      return;
    }
    setPendingDonationsLoading(true);
    setPendingDonationsError('');
    try {
      const response = await fetch(`${GATEWAY}/donations/pending?campaign_id=${encodeURIComponent(campaignId)}`, {
        headers: { Authorization: 'Bearer ' + token },
        signal,
      });
      if (signal?.aborted) return;
      if (!response.ok) {
        setPendingDonationsError((await response.text()) || 'Pembayaran tertunda belum dapat dimuat.');
        return;
      }
      const result = await response.json();
      if (signal?.aborted) return;
      setPendingDonations(Array.isArray(result.payments) ? result.payments : []);
    } catch {
      if (!signal?.aborted) setPendingDonationsError('Tidak dapat terhubung untuk memuat pembayaran tertunda.');
    } finally {
      if (!signal?.aborted) setPendingDonationsLoading(false);
    }
  }

  async function loadDonorDonations(signal) {
    if (!token || userRole !== 'DONATUR') {
      setDonorDonations([]);
      return;
    }
    setDonorDonationsLoading(true);
    setDonorDonationsError('');
    try {
      const response = await fetch(`${GATEWAY}/donations/mine`, {
        headers: { Authorization: 'Bearer ' + token },
        signal,
      });
      if (signal?.aborted) return;
      if (!response.ok) {
        setDonorDonationsError((await response.text()) || 'Riwayat donasi belum dapat dimuat.');
        return;
      }
      const result = await response.json();
      if (signal?.aborted) return;
      setDonorDonations(Array.isArray(result.donations) ? result.donations : []);
    } catch {
      if (!signal?.aborted) setDonorDonationsError('Tidak dapat terhubung untuk memuat riwayat donasi.');
    } finally {
      if (!signal?.aborted) setDonorDonationsLoading(false);
    }
  }

  function resumeDonationPayment(payment) {
    if (!payment || typeof payment.redirect_url !== 'string') return;
    try {
      const paymentURL = new URL(payment.redirect_url);
      if (paymentURL.protocol !== 'https:' || paymentURL.hostname !== 'app.sandbox.midtrans.com') {
        setPendingDonationsError('Alamat pembayaran tersimpan tidak valid.');
        return;
      }
      setResumingDonationReference(payment.reference);
      window.location.assign(paymentURL.toString());
    } catch {
      setPendingDonationsError('Alamat pembayaran tersimpan tidak valid.');
    }
  }

  function confirmCancelDonationPayment(reference) {
    showDialog({
      title: 'Batalkan pembayaran?',
      message: 'Checkout ini akan dibatalkan dan tautannya tidak bisa digunakan lagi. Kamu bisa memulai donasi baru kapan saja.',
      confirmLabel: 'Ya, batalkan',
      confirm: () => cancelDonationPayment(reference),
    });
  }

  async function cancelDonationPayment(reference) {
    setCancellingDonationReference(reference);
    try {
      const response = await fetch(`${GATEWAY}/donations/${encodeURIComponent(reference)}/cancel`, {
        method: 'POST',
        headers: { Authorization: 'Bearer ' + token },
      });
      if (!response.ok) {
        const errorMessage = (await response.text()) || 'Pembayaran belum berhasil dibatalkan. Coba lagi.';
        showDialog({ title: 'Pembayaran belum dibatalkan', message: errorMessage });
        return;
      }

      const result = await response.json();
      if (result.status === 'SETTLEMENT') {
        setDonationPaymentStatus('SETTLEMENT');
        setDonationPaidAmount(Number(result.amount) || 0);
        setDonationMessage('Pembayaran ternyata sudah berhasil diterima. Terima kasih sudah berdonasi!');
        await loadCampaigns();
        showDialog({
          kind: 'success',
          title: 'Pembayaran sudah berhasil',
          message: 'Transaksi ini sudah terkonfirmasi oleh Midtrans, jadi tidak dapat dibatalkan.',
        });
        return;
      }
      if (result.status !== 'FAILED') {
        showDialog({
          title: 'Status pembayaran belum pasti',
          message: 'Pembayaran belum dapat dipastikan batal. Periksa kembali statusnya sebelum mencoba lagi.',
        });
        return;
      }

      setPendingDonations((current) => current.filter((payment) => payment.reference !== reference));
      if (reference === donationReference) {
        setDonationReference('');
        setDonationPaymentStatus('IDLE');
        setDonationMessage('');
        window.history.replaceState({}, '', window.location.pathname);
      }
      const campaignId = donationPageMatch?.[1] || detailCampaign?.id;
      if (campaignId) await loadPendingDonations(campaignId);
      showDialog({
        kind: 'success',
        title: 'Pembayaran dibatalkan',
        message: 'Checkout sudah dibatalkan. Kamu bisa membuat pembayaran baru kapan saja.',
      });
    } catch {
      showDialog({
        title: 'Pembayaran belum dibatalkan',
        message: 'Tidak dapat terhubung untuk membatalkan pembayaran. Periksa koneksi lalu coba lagi.',
      });
    } finally {
      setCancellingDonationReference('');
    }
  }

  async function loadCampaigns() {
    try {
      const result = await gql('{ campaigns { id title description target collected status media { id mediaType contentType } } }');
      if (result.errors) {
        setCampaignsError('Campaign belum dapat dimuat. Coba muat ulang halaman.');
        return [];
      }
      if (result.data?.campaigns) {
        setCampaigns(result.data.campaigns);
        return result.data.campaigns;
      }
      return [];
    } catch {
      setCampaignsError('Tidak dapat terhubung untuk memuat campaign. Periksa koneksi lalu coba lagi.');
      return [];
    } finally {
      setCampaignsLoading(false);
    }
  }

  async function loadPendingCampaigns() {
    const result = await gql('{ pendingCampaigns { id title description target collected status submittedByName submittedByEmail createdAt media { id mediaType contentType } } }');
    if (result.errors) {
      showDialog({ title: 'Campaign tidak dapat dimuat', message: result.errors[0].message });
      return;
    }
    setPendingCampaigns(result.data?.pendingCampaigns || []);
  }

  async function loadPendingCampaignDetail() {
    setPendingCampaignDetailLoading(true);
    setPendingCampaignDetailError('');
    try {
      const result = await gql(`{ pendingCampaign(id: ${Number(adminCampaignId)}) { id title description target collected status submittedByName submittedByEmail createdAt media { id mediaType contentType } } }`);
      if (result.errors?.length) {
        setPendingCampaignDetailError(result.errors.map((error) => error.message).join(' '));
        return;
      }
      setPendingCampaignDetail(result.data?.pendingCampaign || null);
    } catch {
      setPendingCampaignDetailError('Tidak dapat memuat detail campaign. Periksa koneksi lalu coba lagi.');
    } finally {
      setPendingCampaignDetailLoading(false);
    }
  }

  function updatePendingMedia(campaignID, field, value) {
    setPendingMediaSelections((previous) => ({
      ...previous,
      [campaignID]: { ...previous[campaignID], [field]: value },
    }));
    setPendingMediaStatuses((previous) => ({
      ...previous,
      [campaignID]: { loading: false, message: '', error: '' },
    }));
  }

  async function uploadPendingCampaignMedia(campaignID) {
    const selection = pendingMediaSelections[campaignID] || {};
    const photos = selection.photos || [];
    const video = selection.video || null;
    const mediaError = validateCampaignMedia(photos, video);
    if (mediaError) {
      setPendingMediaStatuses((previous) => ({
        ...previous,
        [campaignID]: { loading: false, message: '', error: mediaError },
      }));
      return;
    }

    setPendingMediaStatuses((previous) => ({
      ...previous,
      [campaignID]: { loading: true, message: '', error: '' },
    }));
    try {
      const formData = new FormData();
      photos.forEach((file) => formData.append('photos', file));
      if (video) formData.append('video', video);
      const response = await fetch(`${GATEWAY}/campaigns/${campaignID}/media`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!response.ok) {
        const errorMessage = await response.text();
        setPendingMediaStatuses((previous) => ({
          ...previous,
          [campaignID]: { loading: false, message: '', error: errorMessage || 'Media gagal disimpan.' },
        }));
        return;
      }
      setPendingMediaStatuses((previous) => ({
        ...previous,
        [campaignID]: { loading: false, message: 'Media tersimpan. Campaign dapat disetujui.', error: '' },
      }));
    } catch {
      setPendingMediaStatuses((previous) => ({
        ...previous,
        [campaignID]: { loading: false, message: '', error: 'Tidak dapat mengunggah media. Periksa koneksi lalu coba lagi.' },
      }));
    }
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
          ...(path === 'register' ? { name } : {}),
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
    const campaignTarget = Number(target);
    if (title.trim().length < 3 || title.trim().length > 100) {
      setCampaignError('Judul campaign harus berisi 3–100 karakter.');
      return;
    }
    if (!Number.isSafeInteger(campaignTarget) || campaignTarget <= 0 || campaignTarget > 2147483647) {
      setCampaignError('Target harus berupa bilangan bulat positif maksimal Rp 2.147.483.647.');
      return;
    }
    if (!description.trim() || description.trim().length > 5000) {
      setCampaignError('Deskripsi campaign wajib diisi dan maksimal 5.000 karakter.');
      return;
    }
    const mediaError = validateCampaignMedia(campaignPhotos, campaignVideo);
    if (mediaError) {
      setCampaignError(mediaError);
      return;
    }
    let campaignID = createdCampaignId;
    try {
      setCampaignLoading(true);
      setCampaignError('');
      if (!campaignID) {
        const result = await gql(
          `mutation CreateCampaign($title: String!, $description: String, $target: Int!) {
            createCampaign(title: $title, description: $description, target: $target) {
              id
              title
              status
            }
          }`,
          { title: title.trim(), description: description.trim(), target: campaignTarget },
        );
        if (result.errors?.length) {
          setCampaignError(result.errors.map((error) => error.message).join(' '));
          return;
        }
        campaignID = result.data?.createCampaign?.id;
        if (!campaignID) {
          setCampaignError('Campaign tidak dapat dikirim. Coba lagi.');
          return;
        }
        setCreatedCampaignId(campaignID);
      }

      const formData = new FormData();
      campaignPhotos.forEach((file) => formData.append('photos', file));
      if (campaignVideo) formData.append('video', campaignVideo);
      const uploadResponse = await fetch(`${GATEWAY}/campaigns/${campaignID}/media`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!uploadResponse.ok) {
        const errorMessage = await uploadResponse.text();
        setCampaignError(`Campaign sudah diajukan, tetapi media gagal diunggah${errorMessage ? `: ${errorMessage}` : '.'} Periksa file lalu coba unggah media lagi.`);
        return;
      }

      showDialog({
        kind: 'success',
        title: 'Campaign berhasil diajukan',
        message: 'Campaign menunggu persetujuan Admin sebelum ditampilkan untuk publik.',
        onClose: () => window.location.assign('/'),
      });
    } catch {
      setCampaignError(campaignID
        ? 'Campaign sudah diajukan, tetapi media belum dapat diunggah. Periksa koneksi lalu coba unggah media lagi.'
        : 'Tidak dapat mengirim campaign. Periksa koneksi dan coba lagi.');
    } finally {
      setCampaignLoading(false);
    }
  }

  async function startDonationPayment(event, campaignId) {
    event.preventDefault();
    const amount = Number(donationAmount);
    const campaign = campaigns.find((item) => String(item.id) === String(campaignId));
    const remainingAmount = campaign
      ? Math.max(0, Number(campaign.target) - Number(campaign.collected))
      : null;
    if (donationAmount.trim() === '' || amount === 0) {
      showDialog({
        title: 'Nominal donasi belum diisi',
        message: 'Donasi Rp0 belum bisa diproses. Masukkan nominal minimal Rp1.000 untuk melanjutkan.',
      });
      setDonationError('Masukkan nominal donasi minimal Rp1.000.');
      return;
    }
    if (!Number.isSafeInteger(amount) || amount < 1000) {
      showDialog({
        title: 'Nominal donasi belum valid',
        message: 'Nominal donasi paling sedikit Rp1.000. Silakan ubah nominal lalu coba lagi.',
      });
      setDonationError('Masukkan nominal minimal Rp 1.000.');
      setDonationMessage('');
      return;
    }
    if (remainingAmount !== null && amount > remainingAmount) {
      const message = remainingAmount === 0
        ? 'Target campaign ini sudah terpenuhi. Donasi baru tidak dapat dilanjutkan.'
        : `Sisa kebutuhan campaign hanya Rp ${remainingAmount.toLocaleString('id-ID')}. Kurangi nominal donasi agar tidak melebihi target.`;
      setDonationError(message);
      showDialog({ title: 'Nominal melebihi sisa kebutuhan', message });
      return;
    }
    const normalizedPhone = donorPhone.trim();
    const phoneDigits = normalizedPhone.replace(/\D/g, '').length;
    if (!/^\+?[0-9() -]+$/.test(normalizedPhone) || phoneDigits < 8 || phoneDigits > 15) {
      setDonationError('Masukkan nomor telepon dengan 8–15 angka.');
      setDonationMessage('');
      return;
    }

    try {
      setDonationLoading(true);
      setDonationError('');
      setDonationMessage('');
      const response = await fetch(`${GATEWAY}/donations`, {
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
          donor_phone: normalizedPhone,
        }),
      });
      if (!response.ok) {
        const errorMessage = await response.text() || 'Pembayaran belum dapat dibuat. Coba lagi.';
        setDonationError(errorMessage);
        showDialog({ title: 'Donasi belum dapat diproses', message: errorMessage });
        if (response.status === 400) await loadCampaigns();
        return;
      }
      const result = await response.json();
      if (typeof result.reference !== 'string' || !/^DON-[a-f0-9]{32}$/.test(result.reference) ||
        typeof result.redirect_url !== 'string') {
        setDonationError('Respons pembayaran dari server tidak valid.');
        return;
      }
      const paymentURL = new URL(result.redirect_url);
      if (paymentURL.protocol !== 'https:' || paymentURL.hostname !== 'app.sandbox.midtrans.com') {
        setDonationError('Alamat pembayaran dari provider tidak valid.');
        return;
      }
      setDonationReference(result.reference);
      setDonationPaymentStatus('PENDING');
      setDonationMessage('Mengalihkan ke halaman pembayaran Midtrans...');
      window.location.assign(paymentURL.toString());
    } catch {
      setDonationError('Tidak dapat terhubung untuk membuat pembayaran. Coba lagi.');
    } finally {
      setDonationLoading(false);
    }
  }

  async function submitCampaignQuestion(event, campaignId) {
    event.preventDefault();
    setCampaignQuestionError('');
    setCampaignQuestionMessage('');
    setCampaignQuestionLoading(true);
    try {
      const result = await gql(
        'mutation($campaignID: Int!, $question: String!) { askCampaignQuestion(campaignID: $campaignID, question: $question) { id } }',
        { campaignID: Number(campaignId), question: campaignQuestion },
      );
      if (result.errors?.length) {
        setCampaignQuestionError(result.errors.map((error) => error.message).join(' '));
        return;
      }
      setCampaignQuestion('');
      setCampaignQuestionMessage('Pertanyaanmu berhasil dikirim.');
      setCampaignEngagementRefresh((refresh) => refresh + 1);
    } catch {
      setCampaignQuestionError('Pertanyaan belum dapat dikirim. Periksa koneksi lalu coba lagi.');
    } finally {
      setCampaignQuestionLoading(false);
    }
  }

  async function submitCampaignAnswer(event, questionID) {
    event.preventDefault();
    const answer = campaignAnswerDrafts[questionID] || '';
    setCampaignAnswerError('');
    setCampaignAnswerMessage('');
    setCampaignAnswerLoading(true);
    try {
      const result = await gql(
        'mutation($questionID: Int!, $answer: String!) { answerCampaignQuestion(questionID: $questionID, answer: $answer) { id } }',
        { questionID: Number(questionID), answer },
      );
      if (result.errors?.length) {
        setCampaignAnswerError(result.errors.map((error) => error.message).join(' '));
        return;
      }
      setCampaignAnswerDrafts((drafts) => ({ ...drafts, [questionID]: '' }));
      setCampaignAnswerMessage('Jawaban berhasil dikirim.');
      setCampaignEngagementRefresh((refresh) => refresh + 1);
    } catch {
      setCampaignAnswerError('Jawaban belum dapat dikirim. Periksa koneksi lalu coba lagi.');
    } finally {
      setCampaignAnswerLoading(false);
    }
  }

  async function submitFundUpdate(event, campaignId) {
    event.preventDefault();
    setFundUpdateError('');
    setFundUpdateMessage('');
    setFundUpdateLoading(true);
    const formData = new FormData();
    formData.append('amount', fundUpdateAmount.replace(/\D/g, ''));
    formData.append('purpose', fundUpdatePurpose);
    formData.append('description', fundUpdateDescription);
    if (fundUpdateEvidence) formData.append('evidence', fundUpdateEvidence);
    try {
      const response = await fetch(`${GATEWAY}/campaigns/${campaignId}/fund-updates`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!response.ok) {
        const message = await response.text();
        setFundUpdateError(message || 'Update penggunaan dana gagal disimpan.');
        return;
      }
      setFundUpdateAmount('');
      setFundUpdatePurpose('');
      setFundUpdateDescription('');
      setFundUpdateEvidence(null);
      setFundUpdateMessage('Update penggunaan dana berhasil ditambahkan.');
      setCampaignEngagementRefresh((refresh) => refresh + 1);
      await loadCampaigns();
    } catch {
      setFundUpdateError('Update penggunaan dana belum dapat disimpan. Periksa koneksi lalu coba lagi.');
    } finally {
      setFundUpdateLoading(false);
    }
  }

  async function approveCampaign(id) {
    showDialog({
      title: 'Setujui campaign?',
      message: 'Campaign ini akan dipublikasikan dan dapat dilihat oleh semua orang.',
      confirm: () => submitCampaignApproval(id),
    });
  }

  async function submitCampaignApproval(id) {
    try {
      const result = await gql(`mutation { approveCampaign(id:${id}) { id title status } }`);
      if (result.errors) {
        showDialog({ title: 'Campaign tidak dapat disetujui', message: result.errors[0].message });
        return;
      }
      await loadPendingCampaigns();
      await loadCampaigns();
      showDialog({
        kind: 'success',
        title: 'Campaign dipublikasikan',
        message: 'Campaign berhasil disetujui dan sekarang tampil untuk publik.',
        onClose: () => window.location.assign('/'),
      });
    } catch {
      showDialog({ title: 'Terjadi kesalahan', message: 'Tidak dapat menyetujui campaign. Periksa koneksi lalu coba lagi.' });
    }
  }

  function logout() {
    localStorage.removeItem('token');
    setToken('');
    setUserRole('');
    setPendingCampaigns([]);
    setDonorName('');
    setDonorEmail('');
  }

  useEffect(() => {
    if (!token) return undefined;

    const expiresAt = expirationFromToken(token);
    const expireSession = () => {
      logout();
      setAuthMode('login');
      setAuthError('Sesi login telah berakhir. Silakan masuk kembali.');
    };
    if (!expiresAt || expiresAt <= Date.now()) {
      expireSession();
      return undefined;
    }

    const timeout = window.setTimeout(expireSession, expiresAt - Date.now());
    const checkExpiration = () => {
      if (Date.now() >= expiresAt) expireSession();
    };
    window.addEventListener('focus', checkExpiration);
    document.addEventListener('visibilitychange', checkExpiration);
    return () => {
      window.clearTimeout(timeout);
      window.removeEventListener('focus', checkExpiration);
      document.removeEventListener('visibilitychange', checkExpiration);
    };
  }, [token]);

  useEffect(() => {
    loadCampaigns();
    let active = true;
    let refreshTimer;

    async function checkBackend() {
      try {
        const healthResponse = await fetch(`${GATEWAY}/health`);
        if (!healthResponse.ok) throw new Error('Gateway health check gagal');
        const healthResult = await healthResponse.json();
        if (healthResult.status !== 'ok') throw new Error('Gateway tidak sehat');
        if (!active) return;

        try {
          const registryResponse = await fetch(`${GATEWAY}/registry`);
          if (!registryResponse.ok) throw new Error('Registry tidak tersedia');
          const registry = await registryResponse.json();
          if (active) {
            setHealth({
              gateway: 'online',
              campaign: serviceCounts(registry.campaign),
              notification: serviceCounts(registry.notification),
              registryAvailable: true,
            });
          }
        } catch {
          if (active) setHealth({ gateway: 'online', campaign: null, notification: null, registryAvailable: false });
        }
      } catch {
        if (active) setHealth({ gateway: 'offline', campaign: null, notification: null, registryAvailable: false });
      }
    }

    checkBackend();
    refreshTimer = window.setInterval(checkBackend, 15000);
    return () => {
      active = false;
      window.clearInterval(refreshTimer);
    };
  }, []);

  useEffect(() => {
    if (!isDetailPage || !detailCampaign) {
      setCampaignEngagement(null);
      setCampaignEngagementError('');
      setCampaignEngagementLoading(false);
      return undefined;
    }

    let active = true;
    setCampaignEngagementLoading(true);
    setCampaignEngagementError('');
    gql(`
      query($id: Int!) {
        campaign(id: $id) {
          canManage
          canAnswerQuestions
          donorCount
          questions { id question askedByName createdAt answer answeredByName answeredAt }
          fundUpdates { id amount purpose description createdByName createdAt hasEvidence evidenceContentType }
          donors { name amount createdAt }
        }
      }
    `, { id: Number(detailCampaign.id) })
      .then((result) => {
        if (!active) return;
        if (result.errors?.length) {
          setCampaignEngagementError(result.errors.map((error) => error.message).join(' '));
          return;
        }
        setCampaignEngagement(result.data?.campaign || null);
      })
      .catch(() => {
        if (active) setCampaignEngagementError('Informasi campaign belum dapat dimuat. Coba muat ulang halaman.');
      })
      .finally(() => {
        if (active) setCampaignEngagementLoading(false);
      });

    return () => { active = false; };
  }, [campaignEngagementRefresh, detailCampaign?.collected, detailCampaign?.id, isDetailPage, token]);

  useEffect(() => {
    if (!isDonationPage || !donationReference || !token || campaignsLoading) return;
    setDonationPaymentStatus('PENDING');
    refreshDonationStatus(donationReference);
  }, [campaignsLoading, donationReference, isDonationPage, token]);

  useEffect(() => {
    const campaignId = isDetailPage ? detailCampaign?.id : isDonationPage ? donationCampaignId : null;
    if (!campaignId || !token || userRole !== 'DONATUR' || (isDonationPage && donationReference)) {
      setPendingDonations([]);
      setPendingDonationsLoading(false);
      return undefined;
    }
    const controller = new AbortController();
    loadPendingDonations(campaignId, controller.signal);
    return () => controller.abort();
  }, [detailCampaign?.id, donationCampaignId, donationReference, isDetailPage, isDonationPage, token, userRole]);

  useEffect(() => {
    if (!isDonasikuPage || !token || userRole !== 'DONATUR') {
      setDonorDonations([]);
      setDonorDonationsLoading(false);
      return undefined;
    }
    const controller = new AbortController();
    loadDonorDonations(controller.signal);
    return () => controller.abort();
  }, [isDonasikuPage, token, userRole]);

  useEffect(() => {
    if (token && userRole === 'ADMIN') {
      loadPendingCampaigns();
    } else {
      setPendingCampaigns([]);
    }
  }, [token, userRole]);

  useEffect(() => {
    if (isAdminCampaignPage && token && userRole === 'ADMIN') {
      loadPendingCampaignDetail();
    } else {
      setPendingCampaignDetail(null);
      setPendingCampaignDetailError('');
    }
  }, [adminCampaignId, isAdminCampaignPage, token, userRole]);

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

              {authMode === 'login' && (
                <>
                  <button
                    className="google-login"
                    type="button"
                    onClick={() => window.location.assign(`${GATEWAY}/auth/google`)}
                  >
                    <span className="google-mark" aria-hidden="true">G</span>
                    Masuk dengan Google
                  </button>
                  <div className="auth-divider"><span>atau masuk dengan email</span></div>
                </>
              )}

              <form className="auth-form" onSubmit={(event) => { event.preventDefault(); auth(authMode === 'login' ? 'login' : 'register'); }}>
                {authMode === 'register' && (
                  <>
                    <label htmlFor="auth-name">Nama lengkap</label>
                    <input
                      id="auth-name"
                      type="text"
                      autoComplete="name"
                      minLength="2"
                      maxLength="100"
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      required
                    />
                  </>
                )}
                <label htmlFor="auth-email">Alamat email</label>
                <input
                  id="auth-email"
                  type="email"
                  placeholder="nama@email.com"
                  autoComplete="email"
                  maxLength="150"
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

  if (isAdminCampaignPage) {
    const canReview = userRole === 'ADMIN';
    const campaign = pendingCampaignDetail;
    return (
      <main className="dashboard admin-review-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
            <button className="logout-button" onClick={logout}>Keluar</button>
          </div>
        </header>

        <section className="admin-review-content">
          <a className="back-link" href="/#admin">← Kembali ke persetujuan campaign</a>
          {!canReview && (
            <div className="detail-message" role="alert">
              <span className="section-kicker">AREA ADMIN</span>
              <h1>Akses khusus Admin</h1>
              <p>Hanya Admin yang dapat melihat detail campaign yang menunggu persetujuan.</p>
            </div>
          )}
          {canReview && pendingCampaignDetailLoading && (
            <div className="detail-message" role="status">Memuat detail pengajuan campaign...</div>
          )}
          {canReview && !pendingCampaignDetailLoading && pendingCampaignDetailError && (
            <div className="detail-message" role="alert">
              <span className="section-kicker">DETAIL TIDAK DAPAT DIMUAT</span>
              <h1>Terjadi kendala</h1>
              <p>{pendingCampaignDetailError}</p>
              <button className="admin-review-approve" onClick={loadPendingCampaignDetail}>Coba lagi</button>
            </div>
          )}
          {canReview && !pendingCampaignDetailLoading && !pendingCampaignDetailError && !campaign && (
            <div className="detail-message">
              <span className="section-kicker">PENGAJUAN TIDAK TERSEDIA</span>
              <h1>Campaign tidak ditemukan</h1>
              <p>Campaign mungkin sudah disetujui atau tidak lagi menunggu peninjauan.</p>
            </div>
          )}
          {canReview && campaign && (
            <>
              <div className="admin-review-heading">
                <div>
                  <span className="section-kicker">PENINJAUAN PENGAJUAN · CAMPAIGN #{campaign.id}</span>
                  <h1>{campaign.title}</h1>
                  <p>Periksa cerita dan informasi pengaju sebelum menyetujui campaign.</p>
                </div>
                <span className="campaign-status pending">Menunggu persetujuan</span>
              </div>
              <CampaignMedia campaign={campaign} interactive token={token} review />
              <div className="admin-review-grid">
                <article className="admin-review-story">
                  <span className="section-kicker">CERITA CAMPAIGN</span>
                  <p>{campaign.description || 'Pengaju belum memberikan deskripsi campaign.'}</p>
                </article>
                <aside className="admin-review-facts">
                  <span className="section-kicker">INFORMASI PENGAJUAN</span>
                  <div className="admin-review-target-card">
                    <span>Target donasi</span>
                    <b>Rp {Number(campaign.target).toLocaleString('id-ID')}</b>
                  </div>
                  <dl>
                    <div><dt>Nama pengaju</dt><dd>{campaign.submittedByName || 'Data pengaju tidak tersedia'}</dd></div>
                    <div><dt>Email</dt><dd>{campaign.submittedByEmail || 'Data email tidak tersedia'}</dd></div>
                    <div><dt>Waktu diajukan</dt><dd>{formatDateTime(campaign.createdAt)}</dd></div>
                    <div><dt>Dana terkumpul</dt><dd>Rp {Number(campaign.collected).toLocaleString('id-ID')}</dd></div>
                    <div><dt>Status</dt><dd>Menunggu persetujuan</dd></div>
                  </dl>
                  <button className="admin-review-approve" onClick={() => approveCampaign(campaign.id)}>
                    Setujui dan publikasikan <span aria-hidden="true">→</span>
                  </button>
                </aside>
              </div>
            </>
          )}
        </section>
        <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Area peninjauan Admin · © 2026</span></footer>
      </main>
    );
  }

  if (isDonationPage) {
    const campaign = campaigns.find((item) => String(item.id) === donationCampaignId);
    const campaignCompleted = campaign && (
      campaign.status === 'SELESAI' ||
      (Number(campaign.target) > 0 && Number(campaign.collected) >= Number(campaign.target))
    );
    const remainingDonationAmount = campaign
      ? Math.max(0, Number(campaign.target) - Number(campaign.collected))
      : 0;

    return (
      <main className="dashboard donation-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
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
              <p>Hanya akun Donatur yang dapat melanjutkan pembayaran.</p>
            </div>
          )}
          {campaign && userRole === 'DONATUR' && (
            <>
              <div className="donation-page-heading">
                <span className="section-kicker">DUKUNG CAMPAIGN</span>
                <h1>Lengkapi dukunganmu</h1>
                <p>Isi informasi donatur dan pilih nominal untuk melanjutkan.</p>
              </div>
              {!donationReference && pendingDonations.length > 0 && (
                <section className="pending-payment-list" aria-label="Pembayaran yang belum selesai">
                  <div>
                    <span className="section-kicker">PEMBAYARAN TERTUNDA</span>
                    <h2>Kamu masih punya pembayaran yang belum selesai</h2>
                    <p>Lanjutkan checkout yang sama tanpa membuat transaksi baru.</p>
                  </div>
                  {pendingDonations.map((payment) => (
                    <div className="pending-payment-item" key={payment.reference}>
                      <button
                        className="pending-payment-button"
                        type="button"
                        onClick={() => resumeDonationPayment(payment)}
                        disabled={Boolean(resumingDonationReference || cancellingDonationReference)}
                      >
                        <span>
                          <b>Rp {Number(payment.amount).toLocaleString('id-ID')}</b>
                          <small>{payment.reference}</small>
                        </span>
                        <span>{resumingDonationReference === payment.reference ? 'Membuka...' : 'Lanjutkan pembayaran →'}</span>
                      </button>
                      <button
                        className="pending-payment-cancel"
                        type="button"
                        onClick={() => confirmCancelDonationPayment(payment.reference)}
                        disabled={Boolean(cancellingDonationReference || resumingDonationReference)}
                      >
                        {cancellingDonationReference === payment.reference ? 'Membatalkan...' : 'Batalkan'}
                      </button>
                    </div>
                  ))}
                </section>
              )}
              {pendingDonationsError && userRole === 'DONATUR' && <p className="campaign-feedback error" role="alert">{pendingDonationsError}</p>}
              {pendingDonationsLoading && userRole === 'DONATUR' && !donationReference && <p className="donation-feedback">Memeriksa pembayaran yang belum selesai...</p>}
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
                  {donationReference || donationMessage ? (
                    donationPaymentStatus === 'SETTLEMENT' ? (
                      <div className="donation-success donation-success-settled" role="status">
                        <div className="donation-success-mark" aria-hidden="true">
                          <span>✓</span>
                        </div>
                        <span className="section-kicker">DONASI BERHASIL DISALURKAN</span>
                        <h2>Terima kasih sudah menjadi bagian dari kebaikan ini.</h2>
                        <p className="donation-success-intro">Dukunganmu untuk <b>{campaign.title}</b> sudah kami terima. Setiap kebaikan berarti.</p>
                        <div className="donation-receipt">
                          <span>Nominal donasi</span>
                          <strong>Rp {donationPaidAmount.toLocaleString('id-ID')}</strong>
                          <div className="donation-receipt-divider" />
                          <span className="donation-reference-label">Nomor referensi</span>
                          <div className="donation-reference-value">
                            <code>{donationReference}</code>
                            <button type="button" onClick={copyDonationReference} aria-label="Salin nomor referensi">
                              {donationReferenceCopied ? 'Tersalin' : 'Salin'}
                            </button>
                          </div>
                        </div>
                        <p className="donation-success-note">Pembayaranmu sudah terkonfirmasi dengan aman. Kamu bisa melihat perkembangan campaign kapan saja.</p>
                        {donationError && <p className="donation-feedback error" role="alert">{donationError}</p>}
                        <div className="donation-success-actions">
                          <a className="donation-submit donation-return" href={`/campaign/${campaign.id}`}>
                            Lihat perkembangan campaign <span aria-hidden="true">→</span>
                          </a>
                          <a className="donation-success-secondary" href="/">Jelajahi campaign lainnya</a>
                        </div>
                      </div>
                    ) : (
                      <div className="donation-success donation-success-pending" role="status">
                        <span className={`donation-success-icon ${donationPaymentStatus === 'FAILED' ? 'is-failed' : ''}`} aria-hidden="true">
                          {donationPaymentStatus === 'FAILED' ? '!' : '…'}
                        </span>
                        <span className="section-kicker">
                          {donationPaymentStatus === 'FAILED' ? 'PEMBAYARAN BELUM BERHASIL' : 'MENUNGGU PEMBAYARAN'}
                        </span>
                        <h2>{donationPaymentStatus === 'FAILED' ? 'Pembayaran belum berhasil' : 'Selesaikan pembayaranmu'}</h2>
                        <p>{donationMessage}</p>
                        {donationReference && <small>Referensi pembayaran: {donationReference}</small>}
                        {donationReference && (
                          <>
                            <button className="donation-submit" type="button" onClick={() => refreshDonationStatus()} disabled={donationStatusLoading || Boolean(cancellingDonationReference)}>
                              {donationStatusLoading ? 'Memeriksa...' : 'Periksa status pembayaran'}
                            </button>
                            {donationPaymentStatus === 'PENDING' && (
                              <button
                                className="donation-cancel-button"
                                type="button"
                                onClick={() => confirmCancelDonationPayment(donationReference)}
                                disabled={Boolean(cancellingDonationReference)}
                              >
                                {cancellingDonationReference === donationReference ? 'Membatalkan...' : 'Batalkan pembayaran'}
                              </button>
                            )}
                          </>
                        )}
                        {donationError && <p className="donation-feedback error" role="alert">{donationError}</p>}
                        <a className="donation-submit donation-return" href={`/campaign/${campaign.id}`}>Kembali ke campaign <span aria-hidden="true">→</span></a>
                      </div>
                    )
                  ) : campaignCompleted ? (
                    <div className="donation-success donation-success-pending" role="status">
                      <span className="section-kicker">CAMPAIGN SELESAI</span>
                      <h2>Target donasi sudah terpenuhi</h2>
                      <p>Terima kasih atas kepedulianmu. Campaign ini tidak menerima donasi baru.</p>
                      <a className="donation-submit donation-return" href={`/campaign/${campaign.id}`}>Lihat perkembangan campaign <span aria-hidden="true">→</span></a>
                    </div>
                  ) : (
                    <form className="donation-form donation-checkout-form" onSubmit={(event) => startDonationPayment(event, campaign.id)}>
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
                            disabled={amount > remainingDonationAmount}
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
                          type="text"
                          inputMode="numeric"
                          value={donationAmount ? Number(donationAmount).toLocaleString('id-ID') : ''}
                          onChange={(event) => { setDonationAmount(event.target.value.replace(/\D/g, '')); setDonationError(''); }}
                          required
                        />
                      </div>
                      <p className="donation-feedback">
                        Sisa kebutuhan campaign: <b>Rp {remainingDonationAmount.toLocaleString('id-ID')}</b>
                      </p>

                      <div className="checkout-section-heading donor-info-heading">
                        <span className="form-step">02</span>
                        <div><h2>Informasi donatur</h2><p>Detail ini digunakan untuk membuat pembayaran.</p></div>
                      </div>
                      <div className="donor-fields">
                        <label htmlFor="donor-name">Nama lengkap <span>*</span></label>
                        <input
                          id="donor-name"
                          type="text"
                          autoComplete="name"
                          minLength="2"
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
                          pattern="\+?[0-9() -]{7,29}"
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

                      <p className="donation-privacy-note">Pembayaran diproses di halaman aman Midtrans Sandbox. Data kontak disimpan bersama transaksi dan tidak ditampilkan di daftar donatur. Sandbox adalah uji coba dan tidak memotong uang sungguhan.</p>
                      {donationError && <p className="donation-feedback error" role="alert">{donationError}</p>}
                      <button className="donation-submit" type="submit" disabled={donationLoading}>
                        {donationLoading ? 'Menyiapkan pembayaran...' : 'Lanjutkan ke pembayaran'}
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

  if (isDonasikuPage) {
    const donationsByCampaign = donorDonations.reduce((grouped, donation) => {
      const campaignID = String(donation.campaign_id);
      if (!grouped.has(campaignID)) {
        grouped.set(campaignID, { campaign: donation, donations: [], confirmedAmount: 0 });
      }
      const campaignGroup = grouped.get(campaignID);
      campaignGroup.donations.push(donation);
      if (donation.status === 'SETTLEMENT') campaignGroup.confirmedAmount += Number(donation.amount);
      return grouped;
    }, new Map());
    const confirmedTotal = donorDonations.reduce(
      (total, donation) => total + (donation.status === 'SETTLEMENT' ? Number(donation.amount) : 0),
      0,
    );
    const donorCampaignCount = donationsByCampaign.size;

    return (
      <main className="dashboard donor-progress-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <a className="donor-progress-nav" href="/semua-campaign">Jelajahi campaign</a>
            <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
            <button className="logout-button" onClick={logout}>Keluar</button>
          </div>
        </header>

        <section className="donor-progress-content">
          <a className="back-link" href="/">← Kembali ke beranda</a>
          <div className="donor-progress-heading">
            <span className="section-kicker">DUKUNGANMU BERARTI</span>
            <h1>Perjalanan donasimu</h1>
            <p>Pantau perkembangan campaign yang kamu dukung dan lihat riwayat donasimu.</p>
          </div>

          {userRole !== 'DONATUR' ? (
            <div className="detail-message" role="alert">
              <span className="section-kicker">KHUSUS DONATUR</span>
              <h2>Halaman ini hanya untuk Donatur</h2>
              <p>Masuk dengan akun Donatur untuk melihat riwayat dan perkembangan donasimu.</p>
            </div>
          ) : (
            <>
              {!donorDonationsError && donorDonations.length > 0 && (
                <div className="donor-progress-stats">
                  <article>
                    <span>Total donasi berhasil</span>
                    <strong>Rp {confirmedTotal.toLocaleString('id-ID')}</strong>
                  </article>
                  <article>
                    <span>Campaign yang didukung</span>
                    <strong>{donorCampaignCount}</strong>
                  </article>
                  <article>
                    <span>Transaksi tercatat</span>
                    <strong>{donorDonations.length}</strong>
                  </article>
                </div>
              )}

              {donorDonationsLoading && (
                <div className="detail-message" role="status">Memuat riwayat dan progres donasimu...</div>
              )}
              {donorDonationsError && (
                <div className="detail-message donor-progress-error" role="alert">
                  <p>{donorDonationsError}</p>
                  <button type="button" className="donation-submit" onClick={() => loadDonorDonations()}>
                    Coba lagi
                  </button>
                </div>
              )}
              {!donorDonationsLoading && !donorDonationsError && donorDonations.length === 0 && (
                <div className="donor-progress-empty">
                  <span aria-hidden="true">♡</span>
                  <h2>Belum ada donasi</h2>
                  <p>Donasi yang berhasil kamu lakukan akan muncul di sini bersama progres campaign-nya.</p>
                  <a className="donation-submit" href="/semua-campaign">Jelajahi campaign <span aria-hidden="true">→</span></a>
                </div>
              )}
              {!donorDonationsLoading && !donorDonationsError && donorDonations.length > 0 && (
                <div className="donor-progress-list">
                  {[...donationsByCampaign.values()].map(({ campaign, donations, confirmedAmount }) => {
                    const target = Number(campaign.target);
                    const collected = Number(campaign.collected);
                    const progress = target > 0 ? Math.min(100, Math.max(0, collected / target * 100)) : 0;
                    return (
                      <article className="donor-progress-card" key={campaign.campaign_id}>
                        <div className="donor-progress-card-heading">
                          <div>
                            <span className="section-kicker">CAMPAIGN YANG KAMU DUKUNG</span>
                            <h2>{campaign.campaign_title}</h2>
                          </div>
                          <a href={`/campaign/${campaign.campaign_id}`} aria-label={`Lihat perkembangan ${campaign.campaign_title}`}>
                            Lihat campaign <span aria-hidden="true">→</span>
                          </a>
                        </div>
                        <div className="donor-progress-amounts">
                          <strong>Rp {collected.toLocaleString('id-ID')}</strong>
                          <span>dari Rp {target.toLocaleString('id-ID')}</span>
                          <b>{progress.toLocaleString('id-ID', { maximumFractionDigits: 0 })}%</b>
                        </div>
                        <div
                          className="donor-progress-bar"
                          role="progressbar"
                          aria-label={`Progres ${campaign.campaign_title}`}
                          aria-valuemin="0"
                          aria-valuemax="100"
                          aria-valuenow={Math.round(progress)}
                        >
                          <i style={{ width: `${progress}%` }} />
                        </div>
                        <div className="donor-contribution-total">
                          <span>Total kontribusimu yang berhasil</span>
                          <strong>Rp {confirmedAmount.toLocaleString('id-ID')}</strong>
                        </div>
                        <div className="donor-donation-history">
                          <h3>Riwayat transaksi</h3>
                          {donations.map((donation) => (
                            <div className="donor-donation-row" key={donation.reference}>
                              <div>
                                <strong>Rp {Number(donation.amount).toLocaleString('id-ID')}</strong>
                                <small>{formatDateTime(donation.created_at)} · {donation.reference}</small>
                              </div>
                              <span className={`donor-donation-status ${donation.status.toLowerCase()}`}>
                                {donation.status === 'SETTLEMENT' ? 'Berhasil' : donation.status === 'PENDING' ? 'Menunggu pembayaran' : 'Tidak berhasil'}
                              </span>
                            </div>
                          ))}
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}
            </>
          )}
        </section>
        <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Setiap kebaikan membawa perubahan. · © 2026</span></footer>
      </main>
    );
  }

  if (isDetailPage) {
    const campaign = detailCampaign;
    const campaignCompleted = campaign && (
      campaign.status === 'SELESAI' ||
      (Number(campaign.target) > 0 && Number(campaign.collected) >= Number(campaign.target))
    );
    const progress = campaign && campaign.target > 0
      ? Math.min(100, Math.max(0, Number(campaign.collected) / Number(campaign.target) * 100))
      : 0;
    const fundUpdates = campaignEngagement?.fundUpdates || [];
    const campaignQuestions = campaignEngagement?.questions || [];
    const campaignDonors = campaignEngagement?.donors || [];
    const campaignDonorCount = Number(campaignEngagement?.donorCount || 0);
    const totalSpent = fundUpdates.reduce((sum, update) => sum + Number(update.amount), 0);
    const remainingFunds = Math.max(0, Number(campaign?.collected || 0) - totalSpent);
    const detailTabs = [
      { id: 'story', label: 'Cerita campaign' },
      { id: 'updates', label: `Update (${fundUpdates.length})` },
      { id: 'funds', label: 'Rincian dana' },
      { id: 'donors', label: `Donatur (${campaignDonorCount})` },
      { id: 'questions', label: `Pertanyaan (${campaignQuestions.length})` },
    ];

    return (
      <main className="dashboard campaign-detail-page">
        <header className="dashboard-header">
          <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
            <span className="brand-mark">d.</span>
            <span>donasi<span className="brand-light">bersama</span></span>
          </a>
          <div className="header-account">
            <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
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
              <div className="campaign-detail-overview">
                <div className="campaign-detail-visual">
                  <CampaignMedia campaign={campaign} interactive />
                </div>
                <section className="campaign-detail-summary">
                  <div className="campaign-detail-tags">
                    <span className="campaign-status">{campaign.status}</span>
                    <span className="campaign-id-label">CAMPAIGN #{campaign.id}</span>
                  </div>
                  <h1>{campaign.title}</h1>
                  <p>{campaign.description || 'Dukung cerita dan inisiatif baik ini bersama-sama.'}</p>
                  <div className="campaign-detail-total">
                    <strong>Rp {Number(campaign.collected).toLocaleString('id-ID')}</strong>
                    <span>dari Rp {Number(campaign.target).toLocaleString('id-ID')}</span>
                    <b>{progress.toLocaleString('id-ID', { maximumFractionDigits: 0 })}%</b>
                  </div>
                  <div className="bar" role="progressbar" aria-label={`Dana terkumpul ${campaign.title}`} aria-valuenow={Math.round(progress)} aria-valuemin="0" aria-valuemax="100">
                    <i style={{ width: `${progress}%` }} />
                  </div>
                  <div className="campaign-detail-stats">
                    <span><b>{campaignDonorCount}</b> donatur tercatat</span>
                    <span><b>{fundUpdates.length}</b> update penggunaan dana</span>
                  </div>
                  {campaignCompleted
                    ? <p className="donation-feedback">Campaign ini telah selesai dan tidak menerima donasi baru.</p>
                    : userRole === 'DONATUR'
                    ? <a className="donation-submit donation-cta" href={`/campaign/${campaign.id}/donate`}>Donasi sekarang <span aria-hidden="true">→</span></a>
                    : <p className="donation-feedback">Masuk dengan akun Donatur untuk melanjutkan pembayaran.</p>}
                  {userRole === 'DONATUR' && pendingDonations.length > 0 && (
                    <section className="pending-payment-list campaign-pending-payments" aria-label="Pembayaran yang belum selesai">
                      <div>
                        <span className="section-kicker">PEMBAYARAN TERTUNDA</span>
                        <h2>Lanjutkan pembayaranmu</h2>
                        <p>Checkout ini belum selesai. Kamu bisa melanjutkan transaksi yang sama.</p>
                      </div>
                      {pendingDonations.map((payment) => (
                        <div className="pending-payment-item" key={payment.reference}>
                          <button
                            className="pending-payment-button"
                            type="button"
                            onClick={() => resumeDonationPayment(payment)}
                            disabled={Boolean(resumingDonationReference || cancellingDonationReference)}
                          >
                            <span>
                              <b>Rp {Number(payment.amount).toLocaleString('id-ID')}</b>
                              <small>{payment.reference}</small>
                            </span>
                            <span>{resumingDonationReference === payment.reference ? 'Membuka...' : 'Lanjutkan pembayaran →'}</span>
                          </button>
                          <button
                            className="pending-payment-cancel"
                            type="button"
                            onClick={() => confirmCancelDonationPayment(payment.reference)}
                            disabled={Boolean(cancellingDonationReference || resumingDonationReference)}
                          >
                            {cancellingDonationReference === payment.reference ? 'Membatalkan...' : 'Batalkan'}
                          </button>
                        </div>
                      ))}
                    </section>
                  )}
                  {userRole === 'DONATUR' && pendingDonationsLoading && <p className="donation-feedback">Memeriksa pembayaran yang belum selesai...</p>}
                  {pendingDonationsError && userRole === 'DONATUR' && <p className="campaign-feedback error" role="alert">{pendingDonationsError}</p>}
                  <p className="demo-warning">Mode Sandbox: pembayaran uji coba dan tidak memakai uang sungguhan.</p>
                </section>
              </div>
              <nav className="campaign-detail-tabs" role="tablist" aria-label="Informasi campaign">
                {detailTabs.map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    role="tab"
                    aria-selected={campaignDetailTab === tab.id}
                    className={campaignDetailTab === tab.id ? 'active' : ''}
                    onClick={() => setCampaignDetailTab(tab.id)}
                  >
                    {tab.label}
                  </button>
                ))}
              </nav>
              <section className="campaign-detail-tab-panel" role="tabpanel">
                {campaignEngagementLoading && <div className="detail-message">Memuat informasi campaign...</div>}
                {campaignEngagementError && <p className="campaign-feedback error" role="alert">{campaignEngagementError}</p>}
                {!campaignEngagementLoading && !campaignEngagementError && campaignDetailTab === 'story' && (
                  <article className="campaign-tab-story">
                    <span className="section-kicker">CERITA CAMPAIGN</span>
                    <h2>Mengapa dukunganmu berarti</h2>
                    <p>{campaign.description || 'Setiap dukungan kecil dapat membawa perubahan besar bagi mereka yang membutuhkan.'}</p>
                  </article>
                )}
                {!campaignEngagementLoading && !campaignEngagementError && campaignDetailTab === 'updates' && (
                  <div className="campaign-updates-panel">
                    {campaignEngagement?.canManage && (
                      <form className="campaign-fund-form" onSubmit={(event) => submitFundUpdate(event, campaign.id)}>
                        <div>
                          <span className="section-kicker">TRANSPARANSI PENGGUNAAN DANA</span>
                          <h2>Tambahkan update dana</h2>
                          <p>Sisa dana yang belum dilaporkan: <b>Rp {remainingFunds.toLocaleString('id-ID')}</b></p>
                        </div>
                        <label>
                          Nominal digunakan
                          <div className="fund-currency-input"><span>Rp</span><input
                            type="text"
                            inputMode="numeric"
                            value={fundUpdateAmount ? Number(fundUpdateAmount).toLocaleString('id-ID') : ''}
                            onChange={(event) => setFundUpdateAmount(event.target.value.replace(/\D/g, ''))}
                            placeholder="0"
                            required
                          /></div>
                        </label>
                        <label>Keperluan<input value={fundUpdatePurpose} onChange={(event) => setFundUpdatePurpose(event.target.value)} minLength="3" maxLength="120" placeholder="Contoh: Pembelian perlengkapan sekolah" required /></label>
                        <label>Keterangan<textarea value={fundUpdateDescription} onChange={(event) => setFundUpdateDescription(event.target.value)} minLength="5" maxLength="2000" rows="3" placeholder="Jelaskan penggunaan dana secara singkat dan jelas" required /></label>
                        <label className="fund-evidence-field">Bukti foto <span>Opsional · JPG, PNG, WebP · maksimal 5 MB</span><input type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => setFundUpdateEvidence(event.target.files?.[0] || null)} /></label>
                        {fundUpdateEvidence && <p className="fund-evidence-name">{fundUpdateEvidence.name}</p>}
                        {fundUpdateError && <p className="campaign-feedback error" role="alert">{fundUpdateError}</p>}
                        {fundUpdateMessage && <p className="campaign-feedback success" role="status">{fundUpdateMessage}</p>}
                        <button className="campaign-submit-button" type="submit" disabled={fundUpdateLoading}>{fundUpdateLoading ? 'Menyimpan...' : 'Simpan update penggunaan dana'}</button>
                      </form>
                    )}
                    <div className="campaign-update-list">
                      <div className="campaign-tab-heading"><span className="section-kicker">RIWAYAT PENGGUNAAN</span><h2>Update dana</h2></div>
                      {!fundUpdates.length && <div className="campaign-tab-empty">Belum ada update penggunaan dana untuk campaign ini.</div>}
                      {fundUpdates.map((update) => (
                        <article className="fund-update-card" key={update.id}>
                          <div className="fund-update-card-heading">
                            <span className="fund-update-icon" aria-hidden="true">↗</span>
                            <div><h3>{update.purpose}</h3><small>{formatDateTime(update.createdAt)} · {update.createdByName}</small></div>
                            <strong>Rp {Number(update.amount).toLocaleString('id-ID')}</strong>
                          </div>
                          <p>{update.description}</p>
                          {update.hasEvidence && <a className="fund-evidence-link" href={`${GATEWAY}/campaigns/${campaign.id}/fund-updates/${update.id}/evidence`} target="_blank" rel="noreferrer">Lihat bukti penggunaan dana ↗</a>}
                        </article>
                      ))}
                    </div>
                  </div>
                )}
                {!campaignEngagementLoading && !campaignEngagementError && campaignDetailTab === 'funds' && (
                  <div className="campaign-funds-panel">
                    <div className="funds-summary-grid">
                      <article><span>Total dana terkumpul</span><strong>Rp {Number(campaign.collected).toLocaleString('id-ID')}</strong></article>
                      <article><span>Total dana digunakan</span><strong>Rp {totalSpent.toLocaleString('id-ID')}</strong></article>
                      <article><span>Sisa dana belum dilaporkan</span><strong>Rp {remainingFunds.toLocaleString('id-ID')}</strong></article>
                    </div>
                    <div className="campaign-tab-heading"><span className="section-kicker">RINCIAN PENGELUARAN</span><h2>Dana digunakan untuk apa?</h2></div>
                    {!fundUpdates.length && <div className="campaign-tab-empty">Pengelola belum menambahkan rincian penggunaan dana.</div>}
                    {fundUpdates.map((update) => (
                      <article className="fund-breakdown-row" key={update.id}>
                        <div><b>{update.purpose}</b><small>{update.description}</small><small>{formatDateTime(update.createdAt)} · {update.createdByName}</small></div>
                        <strong>Rp {Number(update.amount).toLocaleString('id-ID')}</strong>
                      </article>
                    ))}
                  </div>
                )}
                {!campaignEngagementLoading && !campaignEngagementError && campaignDetailTab === 'donors' && (
                  <div className="campaign-donors-panel">
                    <div className="campaign-tab-heading"><span className="section-kicker">DUKUNGAN YANG DITERIMA</span><h2>Donatur ({campaignDonorCount})</h2></div>
                    {!campaignDonors.length && <div className="campaign-tab-empty">Belum ada donasi tercatat untuk campaign ini.</div>}
                    {campaignDonors.map((donor, index) => (
                      <article className="campaign-donor-row" key={`${donor.createdAt}-${index}`}>
                        <span className="campaign-donor-avatar" aria-hidden="true">♡</span>
                        <div><b>{donor.name}</b><small>{formatDateTime(donor.createdAt)}</small></div>
                        <strong>Rp {Number(donor.amount).toLocaleString('id-ID')}</strong>
                      </article>
                    ))}
                  </div>
                )}
                {!campaignEngagementLoading && !campaignEngagementError && campaignDetailTab === 'questions' && (
                  <div className="campaign-questions-panel">
                    {userRole === 'DONATUR' && (
                      <form className="campaign-question-form" onSubmit={(event) => submitCampaignQuestion(event, campaign.id)}>
                        <div><span className="section-kicker">PUNYA PERTANYAAN?</span><h2>Tanyakan kepada pengelola</h2></div>
                        <textarea value={campaignQuestion} onChange={(event) => { setCampaignQuestion(event.target.value); setCampaignQuestionError(''); setCampaignQuestionMessage(''); }} minLength="10" maxLength="2000" rows="3" placeholder="Tulis pertanyaan tentang campaign ini..." required />
                        {campaignQuestionError && <p className="campaign-feedback error" role="alert">{campaignQuestionError}</p>}
                        {campaignQuestionMessage && <p className="campaign-feedback success" role="status">{campaignQuestionMessage}</p>}
                        <button className="campaign-submit-button" type="submit" disabled={campaignQuestionLoading}>{campaignQuestionLoading ? 'Mengirim...' : 'Kirim pertanyaan'}</button>
                      </form>
                    )}
                    {userRole !== 'DONATUR' && <p className="campaign-question-login">Masuk dengan akun Donatur untuk mengirim pertanyaan.</p>}
                    {campaignAnswerError && <p className="campaign-feedback error" role="alert">{campaignAnswerError}</p>}
                    {campaignAnswerMessage && <p className="campaign-feedback success" role="status">{campaignAnswerMessage}</p>}
                    <div className="campaign-tab-heading"><span className="section-kicker">TANYA JAWAB</span><h2>Pertanyaan ({campaignQuestions.length})</h2></div>
                    {!campaignQuestions.length && <div className="campaign-tab-empty">Belum ada pertanyaan. Jadilah yang pertama bertanya.</div>}
                    {campaignQuestions.map((question) => (
                      <article className="campaign-question-card" key={question.id}>
                        <div className="campaign-question-meta"><b>{question.askedByName}</b><time>{formatDateTime(question.createdAt)}</time></div>
                        <p>{question.question}</p>
                        {question.answer
                          ? <div className="campaign-answer"><b>Jawaban dari {question.answeredByName}</b><p>{question.answer}</p><small>{formatDateTime(question.answeredAt)}</small></div>
                          : <div className="campaign-question-unanswered">Menunggu jawaban pengaju campaign</div>}
                        {campaignEngagement?.canAnswerQuestions && !question.answer && (
                          <form className="campaign-answer-form" onSubmit={(event) => submitCampaignAnswer(event, question.id)}>
                            <textarea value={campaignAnswerDrafts[question.id] || ''} onChange={(event) => setCampaignAnswerDrafts((drafts) => ({ ...drafts, [question.id]: event.target.value }))} minLength="2" maxLength="2000" rows="2" placeholder="Tulis jawaban..." required />
                            <button className="campaign-submit-button" type="submit" disabled={campaignAnswerLoading}>{campaignAnswerLoading ? 'Mengirim...' : 'Jawab pertanyaan'}</button>
                          </form>
                        )}
                      </article>
                    ))}
                  </div>
                )}
              </section>
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
            <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
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
                required
                maxLength="5000"
                rows="5"
              />
              <small>Deskripsi wajib diisi, maksimal 5.000 karakter.</small>
            </div>
            <div className="form-section-heading target-heading">
              <span className="form-step">02</span>
              <div><h2>Tambahkan media</h2><p>Foto membantu calon donatur memahami campaign-mu.</p></div>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-photos">Foto campaign <span>*</span></label>
              <label className="upload-dropzone" htmlFor="campaign-photos">
                <span className="upload-icon" aria-hidden="true">↑</span>
                <span>
                  <b>{campaignPhotos.length ? `${campaignPhotos.length} foto dipilih` : 'Pilih foto campaign'}</b>
                  <small>{campaignPhotos.length ? campaignPhotos.map((file) => file.name).join(', ') : 'JPG, PNG, atau WebP · maksimal 5 foto, 5 MB per foto'}</small>
                </span>
                <input
                  id="campaign-photos"
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  multiple
                  onChange={(event) => {
                    setCampaignPhotos(Array.from(event.target.files || []));
                    setCampaignError('');
                  }}
                  disabled={campaignLoading}
                  aria-label="Pilih foto campaign"
                />
              </label>
              <small>Minimal 1 foto, maksimal 5 foto. Setiap foto maksimal 5 MB.</small>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-video">Video campaign (opsional)</label>
              <label className="upload-dropzone" htmlFor="campaign-video">
                <span className="upload-icon" aria-hidden="true">▶</span>
                <span>
                  <b>{campaignVideo ? campaignVideo.name : 'Pilih video campaign'}</b>
                  <small>MP4 · maksimal 25 MB</small>
                </span>
                <input
                  id="campaign-video"
                  type="file"
                  accept="video/mp4"
                  onChange={(event) => {
                    setCampaignVideo(event.target.files?.[0] || null);
                    setCampaignError('');
                  }}
                  disabled={campaignLoading}
                  aria-label="Pilih video campaign"
                />
              </label>
            </div>
            <div className="form-section-heading target-heading">
              <span className="form-step">03</span>
              <div><h2>Tentukan target</h2><p>Berapa dana yang dibutuhkan?</p></div>
            </div>
            <div className="field-group">
              <label htmlFor="campaign-target">Target donasi <span>*</span></label>
              <div className="currency-input"><span>Rp</span><input
                id="campaign-target"
                type="text"
                inputMode="numeric"
                placeholder="10.000.000"
                value={target ? Number(target).toLocaleString('id-ID') : ''}
                onChange={(event) => setTarget(event.target.value.replace(/\D/g, ''))}
                required
                aria-describedby="campaign-target-help"
              /></div>
              <small id="campaign-target-help">Tentukan target realistis untuk kebutuhan campaign. Gunakan angka bulat.</small>
            </div>
            {campaignError && <p className="campaign-form-error" role="alert">{campaignError}</p>}
            <div className="form-actions">
              <a href="/">Batal</a>
              <button disabled={campaignLoading}>
                {campaignLoading ? 'Mengirim...' : createdCampaignId ? 'Coba unggah media lagi' : 'Kirim campaign'}
                {!campaignLoading && <span aria-hidden="true">→</span>}
              </button>
            </div>
          </form>
          <aside className="create-sidebar">
            <div className="campaign-preview">
              <div className="preview-art">
                <span>✳</span>
                <small>PRATINJAU CAMPAIGN</small>
              </div>
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
    <main className={`dashboard${isCampaignsPage ? ' campaigns-page' : ''}`}>
      <header className="dashboard-header">
        <a className="dashboard-brand" href="/" aria-label="Donasi Bersama">
          <span className="brand-mark">d.</span>
          <span>donasi<span className="brand-light">bersama</span></span>
        </a>
        <div className="header-account">
          <div className="service-status">
            <span className={`status ${gatewayStatus}`} role="status">
              <i />
              {gatewayStatus === 'checking'
                ? 'Memeriksa sistem...'
                : gatewayStatus === 'offline'
                  ? 'Gateway offline'
                  : gatewayStatus === 'online'
                    ? 'Semua sistem aktif'
                    : health.registryAvailable ? 'Sebagian service offline' : 'Status service tidak tersedia'}
            </span>
            <small>
              Gateway: {health.gateway === 'online' ? 'Aktif' : health.gateway === 'offline' ? 'Offline' : 'Memeriksa'}
              {' · '}Campaign: {health.campaign ? `${health.campaign.healthy}/${health.campaign.total}` : 'Tidak diketahui'}
              {' · '}Notifikasi: {health.notification ? `${health.notification.healthy}/${health.notification.total}` : 'Tidak diketahui'}
            </small>
          </div>
          <AccountIdentity name={accountName} email={accountEmail} role={userRole} />
          {userRole === 'DONATUR' && <a className="donor-progress-nav" href="/donasiku">Donasiku</a>}
          <a className="create-campaign-link" href="/campaign/new">Ajukan campaign <span aria-hidden="true">+</span></a>
          <button className="logout-button" onClick={logout}>Keluar</button>
        </div>
      </header>

      {!isCampaignsPage && <section className="dashboard-hero">
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
      </section>}

      <div className="dashboard-content">
        <section className="campaign-section" id="campaigns">
          <div className="section-heading">
            <div>
              {isCampaignsPage && <a className="back-link campaigns-back-link" href="/">← Kembali ke beranda</a>}
              <span className="section-kicker">{isCampaignsPage ? 'JELAJAHI SEMUA' : 'CERITA YANG MENGGERAKKAN'}</span>
              <h2>{isCampaignsPage ? 'Semua campaign' : 'Temukan campaign'}</h2>
            </div>
            {!isCampaignsPage && activeCampaigns.length > HOME_CAMPAIGNS_COUNT && (
              <a className="campaign-see-all" href="/semua-campaign">Lihat semua <span aria-hidden="true">→</span></a>
            )}
          </div>
          {isCampaignsPage && activeCampaigns.length > 0 && <div className="campaign-tools">
            <span className="campaign-results-count">
              {filteredCampaigns.length === activeCampaigns.length
                ? `${activeCampaigns.length} campaign tersedia`
                : `${filteredCampaigns.length} dari ${activeCampaigns.length} campaign`}
            </span>
            <label className="campaign-search">
              <span aria-hidden="true">⌕</span>
              <input
                type="search"
                value={campaignSearch}
                onChange={(event) => {
                  setCampaignSearch(event.target.value);
                  setCampaignPage(1);
                }}
                placeholder="Cari nama campaign"
                aria-label="Cari campaign berdasarkan nama atau deskripsi"
              />
            </label>
          </div>}
          <div className="cards">
            {visibleCampaigns.map((campaign, index) => (
              <CampaignCard
                campaign={campaign}
                index={isCampaignsPage ? (currentCampaignPage - 1) * CAMPAIGNS_PER_PAGE + index : index}
                key={campaign.id}
              />
            ))}
            {!campaignsLoading && campaignsError && <div className="empty-state" role="alert"><span>!</span><h3>Campaign gagal dimuat</h3><p>{campaignsError}</p></div>}
            {campaignsLoading && !campaigns.length && <div className="empty-state" role="status"><span>…</span><h3>Memuat campaign</h3><p>Sebentar, kami sedang mengambil campaign aktif.</p></div>}
            {!campaignsLoading && !campaignsError && !activeCampaigns.length && !completedCampaigns.length && (
              <div className="empty-state">
                <span>✳</span><h3>Campaign sedang disiapkan</h3>
                <p>Belum ada campaign yang dipublikasikan. Silakan kembali lagi nanti.</p>
              </div>
            )}
            {!campaignsLoading && !campaignsError && !activeCampaigns.length && completedCampaigns.length > 0 && (
              <div className="empty-state">
                <span>✓</span><h3>Semua target campaign telah tercapai</h3>
                <p>Lihat cerita campaign yang sudah selesai di bagian bawah halaman.</p>
                {isCampaignsPage && <a href="/#campaign-selesai">Lihat campaign selesai <span aria-hidden="true">→</span></a>}
              </div>
            )}
            {isCampaignsPage && !campaignsLoading && !campaignsError && activeCampaigns.length > 0 && !filteredCampaigns.length && (
              <div className="empty-state">
                <span>⌕</span><h3>Campaign tidak ditemukan</h3>
                <p>Coba kata kunci lain atau tampilkan kembali semua campaign.</p>
                <button type="button" onClick={() => setCampaignSearch('')}>Hapus pencarian</button>
              </div>
            )}
          </div>
          {!isCampaignsPage && !campaignsLoading && !campaignsError && completedCampaigns.length > 0 && (
            <section className="completed-campaign-section" id="campaign-selesai">
              <div className="section-heading">
                <div>
                  <span className="section-kicker">TERIMA KASIH ATAS KEBAIKANMU</span>
                  <h2>Campaign selesai</h2>
                </div>
                <span className="section-count">{completedCampaigns.length} campaign</span>
              </div>
              <div className="cards">
                {completedCampaigns.map((campaign, index) => (
                  <CampaignCard campaign={campaign} index={index} completed key={campaign.id} />
                ))}
              </div>
            </section>
          )}
          {campaignPageCount > 1 && (
            <nav className="campaign-pagination" aria-label="Halaman campaign">
              <span>
                Menampilkan {(currentCampaignPage - 1) * CAMPAIGNS_PER_PAGE + 1}–{Math.min(currentCampaignPage * CAMPAIGNS_PER_PAGE, filteredCampaigns.length)} dari {filteredCampaigns.length}
              </span>
              <div className="campaign-pagination-controls">
                <button
                  type="button"
                  onClick={() => {
                    setCampaignPage(currentCampaignPage - 1);
                    document.getElementById('campaigns')?.scrollIntoView({ block: 'start' });
                  }}
                  disabled={currentCampaignPage === 1}
                >
                  Sebelumnya
                </button>
                <span>Halaman {currentCampaignPage} dari {campaignPageCount}</span>
                <button
                  type="button"
                  onClick={() => {
                    setCampaignPage(currentCampaignPage + 1);
                    document.getElementById('campaigns')?.scrollIntoView({ block: 'start' });
                  }}
                  disabled={currentCampaignPage === campaignPageCount}
                >
                  Berikutnya
                </button>
              </div>
            </nav>
          )}
        </section>

      </div>

      {token && userRole === 'ADMIN' && !isCampaignsPage && (
        <section className="create approval-section" id="admin">
          <div className="section-heading">
            <div><span className="section-kicker">AREA ADMIN</span><h2>Persetujuan campaign</h2></div>
            <span className="section-count">{pendingCampaigns.length} menunggu</span>
          </div>
          <div className="pending-list">
            {visiblePendingCampaigns.map((campaign) => (
              <article className="pending-row" key={campaign.id}>
                <div className="pending-row-info">
                  <span className="pending-row-id">#{campaign.id}</span>
                  <div className="pending-row-title">
                    <h3>{campaign.title}</h3>
                    <p>{campaign.submittedByName || 'Data pengaju tidak tersedia'} · {formatDateTime(campaign.createdAt)}</p>
                  </div>
                </div>
                <div className="pending-row-target">Rp {Number(campaign.target).toLocaleString('id-ID')}</div>
                <div className="pending-row-actions">
                  <a href={`/admin/campaign/${campaign.id}`} target="_blank" rel="noreferrer">Lihat detail</a>
                  <button type="button" onClick={() => approveCampaign(campaign.id)}>Setujui</button>
                </div>
                <details className="pending-media-disclosure">
                  <summary>Tambah media (opsional)</summary>
                  <div className="pending-media-form">
                    <label htmlFor={`pending-photos-${campaign.id}`}>Tambah foto atau video sebelum disetujui</label>
                    <label className="upload-dropzone" htmlFor={`pending-photos-${campaign.id}`}>
                      <span className="upload-icon" aria-hidden="true">↑</span>
                      <span>
                        <b>{pendingMediaSelections[campaign.id]?.photos?.length
                          ? `${pendingMediaSelections[campaign.id].photos.length} foto dipilih`
                          : 'Pilih foto'}
                        </b>
                        <small>{pendingMediaSelections[campaign.id]?.photos?.map((file) => file.name).join(', ')
                          || 'JPG, PNG, atau WebP · maksimal 5 foto'}
                        </small>
                      </span>
                      <input
                        id={`pending-photos-${campaign.id}`}
                        type="file"
                        accept="image/jpeg,image/png,image/webp"
                        multiple
                        onChange={(event) => updatePendingMedia(campaign.id, 'photos', Array.from(event.target.files || []))}
                        disabled={pendingMediaStatuses[campaign.id]?.loading || Boolean(pendingMediaStatuses[campaign.id]?.message)}
                        aria-label={`Pilih foto untuk campaign ${campaign.title}`}
                      />
                    </label>
                    <label className="upload-dropzone" htmlFor={`pending-video-${campaign.id}`}>
                      <span className="upload-icon" aria-hidden="true">▶</span>
                      <span>
                        <b>{pendingMediaSelections[campaign.id]?.video?.name || 'Pilih video (opsional)'}</b>
                        <small>MP4 · maksimal 25 MB</small>
                      </span>
                      <input
                        id={`pending-video-${campaign.id}`}
                        type="file"
                        accept="video/mp4"
                        onChange={(event) => updatePendingMedia(campaign.id, 'video', event.target.files?.[0] || null)}
                        disabled={pendingMediaStatuses[campaign.id]?.loading || Boolean(pendingMediaStatuses[campaign.id]?.message)}
                        aria-label={`Pilih video untuk campaign ${campaign.title}`}
                      />
                    </label>
                    {pendingMediaStatuses[campaign.id]?.error && (
                      <p className="pending-media-feedback error" role="alert">{pendingMediaStatuses[campaign.id].error}</p>
                    )}
                    {pendingMediaStatuses[campaign.id]?.message && (
                      <p className="pending-media-feedback success" role="status">{pendingMediaStatuses[campaign.id].message}</p>
                    )}
                    {!pendingMediaStatuses[campaign.id]?.message && (
                      <button
                        type="button"
                        onClick={() => uploadPendingCampaignMedia(campaign.id)}
                        disabled={pendingMediaStatuses[campaign.id]?.loading}
                      >
                        {pendingMediaStatuses[campaign.id]?.loading ? 'Mengunggah...' : 'Simpan media'}
                      </button>
                    )}
                  </div>
                </details>
              </article>
            ))}
            {!pendingCampaigns.length && <div className="empty-state"><span>✓</span><h3>Semua sudah ditinjau</h3><p>Tidak ada campaign yang menunggu persetujuan.</p></div>}
          </div>
          {pendingCampaignPageCount > 1 && (
            <nav className="pending-pagination" aria-label="Halaman persetujuan campaign">
              <button
                type="button"
                onClick={() => setPendingCampaignPage(currentPendingCampaignPage - 1)}
                disabled={currentPendingCampaignPage === 1}
              >
                Sebelumnya
              </button>
              <span>Halaman {currentPendingCampaignPage} dari {pendingCampaignPageCount}</span>
              <button
                type="button"
                onClick={() => setPendingCampaignPage(currentPendingCampaignPage + 1)}
                disabled={currentPendingCampaignPage === pendingCampaignPageCount}
              >
                Berikutnya
              </button>
            </nav>
          )}
        </section>
      )}

      <footer><span className="footer-brand">donasi<span>bersama</span></span><span>Berbuat baik, bersama. · © 2026</span></footer>
    </main>
  );
}

function App() {
  const [dialog, setDialog] = useState(null);
  const showDialog = useCallback((nextDialog) => setDialog(nextDialog), []);
  const closeDialog = useCallback(() => {
    const onClose = dialog?.onClose;
    setDialog(null);
    onClose?.();
  }, [dialog]);

  return (
    <>
      <AppContent showDialog={showDialog} />
      <AppDialog dialog={dialog} onClose={closeDialog} />
    </>
  );
}

createRoot(document.getElementById('root')).render(<App />);
