import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import { setLocale } from '../i18n/index.svelte.js';
import StartupScreen from './StartupScreen.svelte';

describe('StartupScreen', () => {
  let component;
  let target;

  beforeEach(() => {
    setLocale('en');
    target = document.createElement('div');
    document.body.appendChild(target);
  });

  afterEach(async () => {
    if (component) await unmount(component);
    target.remove();
    component = null;
  });

  function render(props = {}) {
    component = mount(StartupScreen, {
      target,
      props: {
        theme: { app_name: 'Test Observer', logo_url: '' },
        status: { stage: 'contacting_server' },
        ...props,
      },
    });
  }

  it('shows the current startup stage and completed steps', () => {
    render({ status: { stage: 'migrating' } });

    expect(target.querySelector('h1').textContent).toBe('Test Observer');
    expect(target.querySelector('[role="status"]').textContent).toContain(
      'Checking and updating the database',
    );
    expect(
      [...target.querySelectorAll('.startup-steps li')].map((step) => step.dataset.state),
    ).toEqual(['complete', 'complete', 'active', 'pending']);
  });

  it('exposes real ClickHouse download progress', () => {
    render({
      status: {
        stage: 'downloading',
        percent: 42,
        bytes_downloaded: 420,
        total_bytes: 1000,
      },
    });

    const progress = target.querySelector('[role="progressbar"]');
    expect(progress.getAttribute('aria-valuenow')).toBe('42');
    expect(target.querySelector('[role="status"]').textContent).toContain('42%');
  });

  it('shows the failing stage and backend error without requiring logs', () => {
    render({
      status: {
        stage: 'migrating',
        error: 'migration 12 failed',
      },
    });

    expect(target.querySelector('.startup-steps li[data-state="error"]').textContent).toContain(
      'Prepare the database',
    );
    expect(target.querySelector('[role="alert"]').textContent).toContain('migration 12 failed');
    expect(target.querySelector('[role="alert"]').textContent).toContain(
      'Restart CrawlObserver to retry',
    );
  });

  it('keeps retrying a temporary server connection failure', () => {
    const onretry = vi.fn();
    render({
      connectionError: 'Failed to fetch',
      onretry,
    });

    expect(target.querySelector('[role="status"]').textContent).toContain('Retrying automatically');
    target.querySelector('.startup-warning button').click();
    expect(onretry).toHaveBeenCalledOnce();
  });
});
