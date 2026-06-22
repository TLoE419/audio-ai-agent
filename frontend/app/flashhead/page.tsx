const FLASHHEAD_URL =
  process.env.NEXT_PUBLIC_FLASHHEAD_URL ??
  process.env.NEXT_PUBLIC_OPENAVATAR_URL ??
  "http://127.0.0.1:8282/";

export default function FlashHeadPage() {
  return (
    <main className="voice-shell is-live">
      <section className="voice-panel is-live" aria-label="FlashHead OpenAvatarChat">
        <header className="voice-header">
          <div className="brand">
            <span className="brand-mark" aria-hidden="true" />
            <span>FlashHead</span>
          </div>
          <div className="header-actions">
            <a className="header-link" href="/">
              Home
            </a>
          </div>
        </header>

        <div className="mode-pages">
          <section className="mode-page live-page" aria-label="FlashHead embedded page">
            <iframe
              className="openavatar-frame"
              src={FLASHHEAD_URL}
              title="FlashHead OpenAvatarChat"
              allow="camera; microphone; autoplay; clipboard-read; clipboard-write; fullscreen"
              allowFullScreen
            />
          </section>
        </div>
      </section>
    </main>
  );
}
