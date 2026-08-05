const REPOSITORY_URL = "https://github.com/Willxup/flowlens";
const PROFILE_URL = "https://github.com/Willxup";

export function footerVersionLabel(version?: string): string | undefined {
  const trimmed = version?.trim();
  return trimmed ? `Version: ${trimmed}` : undefined;
}

export function AppFooter({ version }: { version?: string }) {
  const versionLabel = footerVersionLabel(version);

  return (
    <footer className="app-footer">
      <div className="app-footer-line app-footer-meta">
        <span>© 2026</span>
        <a href={REPOSITORY_URL} target="_blank" rel="noreferrer">
          FlowLens
        </a>
        <span aria-hidden="true">·</span>
        <a
          href={`${REPOSITORY_URL}/blob/main/LICENSE`}
          target="_blank"
          rel="noreferrer"
        >
          License
        </a>
      </div>
      <div className="app-footer-line app-footer-powered">
        <span>Powered By</span>
        <a
          href={PROFILE_URL}
          target="_blank"
          rel="noreferrer"
          aria-label="Willxup GitHub 主页"
        >
          <GitHubIcon />
          <span>Willxup</span>
        </a>
        {versionLabel ? (
          <>
            <span className="app-footer-version-separator" aria-hidden="true">
              ·
            </span>
            <span className="app-footer-version">{versionLabel}</span>
          </>
        ) : null}
      </div>
    </footer>
  );
}

function GitHubIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4" />
      <path d="M9 18c-4.51 2-5-2-7-2" />
    </svg>
  );
}
