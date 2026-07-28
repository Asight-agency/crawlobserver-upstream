import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, tick, unmount } from 'svelte';
import SessionsList from './SessionsList.svelte';

describe('SessionsList', () => {
  let component;
  let target;
  let onselectsession;
  let onresume;

  const session = {
    ID: 'session-1',
    SeedURLs: ['https://example.com'],
    Status: 'completed',
    PagesCrawled: 42,
    StartedAt: new Date().toISOString(),
    ProjectID: '',
    is_running: false,
    is_queued: false,
  };

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    onselectsession = vi.fn();
    onresume = vi.fn();
    component = mount(SessionsList, {
      target,
      props: {
        sessions: [session],
        projects: [],
        liveProgress: {},
        sessionStorageMap: {},
        loading: false,
        onselectsession,
        onresume,
      },
    });
  });

  afterEach(async () => {
    await unmount(component);
    target.remove();
  });

  it('opens a session with mouse and keyboard', () => {
    const row = target.querySelector('.session-row');

    row.click();
    row.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    row.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }));

    expect(onselectsession).toHaveBeenCalledTimes(3);
    expect(onselectsession).toHaveBeenLastCalledWith(session);
  });

  it('does not open the session when selecting its checkbox', async () => {
    const checkbox = target.querySelector('input[type="checkbox"]');
    target.querySelector('.session-checkbox').click();
    await tick();

    expect(checkbox.checked).toBe(true);
    expect(onselectsession).not.toHaveBeenCalled();
  });

  it('keeps row and action-button interactions separate', () => {
    const resume = [...target.querySelectorAll('button')].find((button) =>
      button.textContent.includes('Resume'),
    );
    resume.click();

    expect(onresume).toHaveBeenCalledWith('session-1');
    expect(onselectsession).not.toHaveBeenCalled();
  });
});
