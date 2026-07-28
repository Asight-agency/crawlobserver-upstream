<script>
  import { t } from '../i18n/index.svelte.js';
  import { fmtSize } from '../utils.js';

  let {
    theme = { app_name: 'CrawlObserver', logo_url: '' },
    status = { stage: 'loading_theme' },
    connectionError = '',
    onretry,
  } = $props();

  const steps = [
    { id: 'server', label: 'startup.stepServer' },
    { id: 'clickhouse', label: 'startup.stepClickHouse' },
    { id: 'database', label: 'startup.stepDatabase' },
    { id: 'workspace', label: 'startup.stepWorkspace' },
  ];

  const stageIndexes = {
    loading_theme: 0,
    contacting_server: 0,
    starting: 1,
    detecting: 1,
    waiting_for_clickhouse: 1,
    downloading: 1,
    starting_clickhouse: 1,
    connecting_clickhouse: 1,
    migrating: 2,
    connecting_database: 2,
    local_storage: 3,
    finalizing: 3,
    initializing_telemetry: 3,
    opening_app: 3,
    ready: 4,
  };

  const stageLabels = {
    loading_theme: 'startup.loadingTheme',
    contacting_server: 'startup.contactingServer',
    starting: 'startup.startingServices',
    detecting: 'startup.detectingClickHouse',
    waiting_for_clickhouse: 'startup.waitingForClickHouse',
    downloading: 'startup.downloadingClickHouse',
    starting_clickhouse: 'startup.startingClickHouse',
    connecting_clickhouse: 'startup.connectingClickHouse',
    migrating: 'startup.migratingDatabase',
    connecting_database: 'startup.connectingDatabase',
    local_storage: 'startup.openingLocalStorage',
    finalizing: 'startup.finalizing',
    initializing_telemetry: 'startup.initializingTelemetry',
    opening_app: 'startup.openingApp',
    ready: 'startup.ready',
  };

  let stage = $derived(status?.stage || 'contacting_server');
  let activeIndex = $derived(stageIndexes[stage] ?? 0);
  let hasError = $derived(Boolean(status?.error));
  let hasConnectionIssue = $derived(Boolean(connectionError));
  let errorMessage = $derived(status?.error || connectionError);
  let stageLabel = $derived(t(stageLabels[stage] || 'startup.startingServices'));
  let downloadPercent = $derived(Math.max(0, Math.min(100, Number(status?.percent || 0))));

  function stepState(index) {
    if (hasError && index === Math.min(activeIndex, steps.length - 1)) return 'error';
    if (activeIndex >= steps.length || index < activeIndex) return 'complete';
    if (index === activeIndex) return 'active';
    return 'pending';
  }
</script>

<main class="startup-screen" aria-labelledby="startup-title">
  <section class="startup-card">
    <div class="startup-brand">
      <img src={theme.logo_url || '/favicon.svg'} alt="" class="startup-logo" />
      <div>
        <h1 id="startup-title">{theme.app_name || 'CrawlObserver'}</h1>
        <p>{hasError ? t('startup.failed') : t('startup.title')}</p>
      </div>
    </div>

    <div
      class="startup-current"
      class:startup-current-error={hasError}
      role="status"
      aria-live="polite"
    >
      {#if hasError}
        <span class="startup-error-icon" aria-hidden="true">!</span>
      {:else}
        <span class="startup-spinner" aria-hidden="true"></span>
      {/if}
      <div>
        <strong>{hasError ? t('startup.unableToStart') : stageLabel}</strong>
        {#if stage === 'downloading' && status.total_bytes > 0}
          <span>
            {downloadPercent}% · {fmtSize(status.bytes_downloaded)} / {fmtSize(status.total_bytes)}
          </span>
        {:else if connectionError}
          <span>{t('startup.retrying')}</span>
        {:else if hasError}
          <span>{t('startup.errorStageStopped')}</span>
        {:else}
          <span>{t('startup.pleaseWait')}</span>
        {/if}
      </div>
    </div>

    {#if stage === 'downloading' && status.total_bytes > 0 && !hasError}
      <div
        class="startup-progress"
        role="progressbar"
        aria-label={t('startup.downloadingClickHouse')}
        aria-valuemin="0"
        aria-valuemax="100"
        aria-valuenow={downloadPercent}
      >
        <span style:width={`${downloadPercent}%`}></span>
      </div>
    {/if}

    <ol class="startup-steps">
      {#each steps as item, index}
        {@const state = stepState(index)}
        <li
          data-state={state}
          class:step-active={state === 'active'}
          class:step-error={state === 'error'}
        >
          <span class="step-marker" aria-hidden="true">
            {#if state === 'complete'}
              <svg viewBox="0 0 20 20">
                <path
                  d="m5 10 3 3 7-7"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            {:else if state === 'error'}
              !
            {:else}
              {index + 1}
            {/if}
          </span>
          <span>{t(item.label)}</span>
        </li>
      {/each}
    </ol>

    {#if hasError}
      <div class="startup-error" role="alert">
        <p>{errorMessage}</p>
        <small>{t('startup.restartHint')}</small>
      </div>
    {:else if hasConnectionIssue}
      <div class="startup-error startup-warning" role="alert">
        <p>{errorMessage}</p>
        <small>{t('startup.connectionHint')}</small>
        {#if onretry}
          <button class="btn btn-primary btn-sm" onclick={() => onretry()}>
            {t('startup.retryNow')}
          </button>
        {/if}
      </div>
    {/if}
  </section>
</main>

<style>
  .startup-screen {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 32px;
    background: radial-gradient(circle at 20% 15%, var(--accent-light), transparent 34%), var(--bg);
    color: var(--text);
  }

  .startup-card {
    width: min(520px, 100%);
    padding: 34px;
    background: color-mix(in srgb, var(--bg-card) 96%, transparent);
    border: 1px solid var(--border);
    border-radius: 18px;
    box-shadow: var(--shadow-md);
  }

  .startup-brand {
    display: flex;
    align-items: center;
    gap: 15px;
    margin-bottom: 30px;
  }

  .startup-logo {
    width: 48px;
    height: 48px;
    flex: 0 0 auto;
    border-radius: 13px;
    object-fit: contain;
  }

  h1 {
    font-family: 'Nunito Sans', sans-serif;
    font-size: 22px;
    line-height: 1.2;
    letter-spacing: -0.02em;
  }

  .startup-brand p {
    margin-top: 3px;
    color: var(--text-muted);
    font-size: 13px;
  }

  .startup-current {
    display: flex;
    align-items: center;
    gap: 13px;
    min-height: 68px;
    padding: 14px 16px;
    background: var(--accent-light);
    border-radius: var(--radius);
  }

  .startup-current-error {
    background: var(--error-bg);
  }

  .startup-current div {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 3px;
  }

  .startup-current strong {
    font-size: 14px;
    font-weight: 650;
  }

  .startup-current span:not(.startup-spinner):not(.startup-error-icon) {
    color: var(--text-secondary);
    font-size: 12px;
  }

  .startup-spinner {
    width: 22px;
    height: 22px;
    flex: 0 0 auto;
    border: 2px solid color-mix(in srgb, var(--accent) 22%, transparent);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: startup-spin 0.8s linear infinite;
  }

  .startup-error-icon {
    display: grid;
    width: 22px;
    height: 22px;
    flex: 0 0 auto;
    place-items: center;
    border-radius: 50%;
    background: var(--error);
    color: white;
    font-size: 13px;
    font-weight: 800;
  }

  .startup-progress {
    height: 5px;
    margin: 12px 2px 0;
    overflow: hidden;
    background: var(--border-light);
    border-radius: 999px;
  }

  .startup-progress span {
    display: block;
    height: 100%;
    background: var(--accent);
    border-radius: inherit;
    transition: width 0.25s ease;
  }

  .startup-steps {
    display: grid;
    gap: 0;
    margin: 28px 0 0;
    list-style: none;
  }

  .startup-steps li {
    position: relative;
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 42px;
    color: var(--text-muted);
    font-size: 13px;
  }

  .startup-steps li:not(:last-child)::after {
    position: absolute;
    top: 31px;
    bottom: -11px;
    left: 12px;
    width: 1px;
    background: var(--border);
    content: '';
  }

  .startup-steps li[data-state='complete'] {
    color: var(--text-secondary);
  }

  .startup-steps li[data-state='complete']::after {
    background: color-mix(in srgb, var(--success) 55%, var(--border));
  }

  .step-active {
    color: var(--text);
    font-weight: 600;
  }

  .step-error {
    color: var(--error);
    font-weight: 600;
  }

  .step-marker {
    z-index: 1;
    display: grid;
    width: 25px;
    height: 25px;
    flex: 0 0 auto;
    place-items: center;
    border: 1px solid var(--border);
    border-radius: 50%;
    background: var(--bg-card);
    font-size: 11px;
    font-weight: 700;
  }

  li[data-state='complete'] .step-marker {
    border-color: color-mix(in srgb, var(--success) 45%, var(--border));
    background: var(--success-bg);
    color: var(--success);
  }

  li[data-state='complete'] svg {
    width: 15px;
    height: 15px;
  }

  .step-active .step-marker {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-text);
    box-shadow: 0 0 0 4px var(--accent-light);
  }

  .step-error .step-marker {
    border-color: var(--error);
    background: var(--error);
    color: white;
  }

  .startup-error {
    display: grid;
    gap: 8px;
    margin-top: 22px;
    padding: 14px 16px;
    border: 1px solid color-mix(in srgb, var(--error) 22%, var(--border));
    border-radius: var(--radius);
    background: var(--error-bg);
  }

  .startup-error p {
    overflow-wrap: anywhere;
    color: var(--error);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
  }

  .startup-error small {
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .startup-warning {
    border-color: color-mix(in srgb, var(--warning) 28%, var(--border));
    background: var(--warning-bg);
  }

  .startup-warning p {
    color: var(--text-secondary);
  }

  .startup-error .btn {
    justify-self: start;
    margin-top: 3px;
  }

  @keyframes startup-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (max-width: 600px) {
    .startup-screen {
      padding: 18px;
    }

    .startup-card {
      padding: 24px;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .startup-spinner {
      animation-duration: 1.8s;
    }

    .startup-progress span {
      transition: none;
    }
  }
</style>
