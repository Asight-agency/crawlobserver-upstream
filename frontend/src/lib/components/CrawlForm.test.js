import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, tick, unmount } from 'svelte';
import { checkIP, getExtractorSets, resumeCrawl, retryFailed, startCrawl } from '../api.js';
import CrawlForm from './CrawlForm.svelte';

vi.mock('../api.js', () => ({
  checkIP: vi.fn(),
  getExtractorSets: vi.fn(),
  resumeCrawl: vi.fn(),
  retryFailed: vi.fn(),
  startCrawl: vi.fn(),
}));

describe('CrawlForm', () => {
  let component;
  let target;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    vi.clearAllMocks();
    getExtractorSets.mockResolvedValue([]);
    startCrawl.mockResolvedValue({});
    resumeCrawl.mockResolvedValue({});
    retryFailed.mockResolvedValue({});
    checkIP.mockResolvedValue({ ip: '203.0.113.10' });
  });

  afterEach(async () => {
    if (component) await unmount(component);
    target.remove();
    component = null;
  });

  function render(props = {}) {
    component = mount(CrawlForm, {
      target,
      props: {
        mode: 'new',
        projects: [],
        ...props,
      },
    });
  }

  async function setInput(selector, value) {
    const input = target.querySelector(selector);
    input.value = value;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await tick();
    return input;
  }

  function submitButton() {
    return target.querySelector('.form-actions .btn-primary');
  }

  it('normalizes seed URLs and starts a crawl with the form defaults', async () => {
    const onsubmit = vi.fn();
    render({ onsubmit });
    expect(submitButton().disabled).toBe(true);

    await setInput('#cf-seeds', 'example.com\nhttps://www.example.org/path');
    expect(submitButton().disabled).toBe(false);
    submitButton().click();

    await vi.waitFor(() => expect(startCrawl).toHaveBeenCalledOnce());
    expect(startCrawl).toHaveBeenCalledWith(
      ['http://example.com', 'https://www.example.org/path'],
      expect.objectContaining({
        workers: 10,
        delay: '1000ms',
        max_pages: 0,
        max_depth: 0,
        crawl_scope: 'host',
        fetch_sitemaps: true,
      }),
    );
    await vi.waitFor(() => expect(onsubmit).toHaveBeenCalledOnce());
  });

  it('restores persisted crawler settings when resuming a session', async () => {
    const session = {
      ID: 'session-1',
      SeedURLs: ['https://example.com'],
      ProjectID: 'project-1',
      Config: JSON.stringify({
        Crawler: {
          Workers: 7,
          Delay: 250_000_000,
          MaxPages: 500,
          MaxDepth: 4,
          CrawlScope: 'domain',
          StoreHTML: true,
        },
      }),
    };
    render({ mode: 'resume', session });

    expect(target.querySelector('#cf-workers').value).toBe('7');
    expect(target.querySelector('#cf-delay').value).toBe('250');
    expect(target.querySelector('#cf-seeds').disabled).toBe(true);

    submitButton().click();
    await vi.waitFor(() => expect(resumeCrawl).toHaveBeenCalledOnce());
    expect(resumeCrawl).toHaveBeenCalledWith(
      'session-1',
      expect.objectContaining({
        workers: 7,
        delay: '250ms',
        max_pages: 500,
        max_depth: 4,
        crawl_scope: 'domain',
        project_id: 'project-1',
        store_html: true,
      }),
    );
  });

  it('reports API failures without completing the form', async () => {
    const onerror = vi.fn();
    const onsubmit = vi.fn();
    startCrawl.mockRejectedValue(new Error('crawl failed'));
    render({ onerror, onsubmit });
    await setInput('#cf-seeds', 'example.com');

    submitButton().click();
    await vi.waitFor(() => expect(onerror).toHaveBeenCalledWith('crawl failed'));
    expect(onsubmit).not.toHaveBeenCalled();
    expect(submitButton().disabled).toBe(false);
  });
});
