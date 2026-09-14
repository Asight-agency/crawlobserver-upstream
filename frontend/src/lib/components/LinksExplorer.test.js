import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, tick, unmount } from 'svelte';
import { getInternalLinks } from '../api.js';
import { downloadCSV } from '../utils.js';
import LinksExplorer from './LinksExplorer.svelte';

vi.mock('../api.js', async (importOriginal) => ({
  ...(await importOriginal()),
  getInternalLinks: vi.fn(),
  getBacklinksTop: vi.fn(),
}));

vi.mock('../utils.js', async (importOriginal) => ({
  ...(await importOriginal()),
  downloadCSV: vi.fn(),
}));

const link = {
  SourceURL: 'https://example.com/page',
  TargetURL: 'https://example.com/products',
  AnchorText: 'Products',
  Tag: 'a',
  Landmark: 'nav',
  XPath: '/html/body/nav/ul/li[1]/a',
  Depth: 5,
  DocumentIndex: 3,
  BlockSignature: '1234567890123456789',
};

describe('LinksExplorer', () => {
  let component;
  let target;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    vi.clearAllMocks();
    getInternalLinks.mockResolvedValue([link]);
  });

  afterEach(async () => {
    if (component) await unmount(component);
    target.remove();
    component = null;
  });

  async function render() {
    component = mount(LinksExplorer, { target, props: { sessionId: 'sess-1' } });
    await tick();
    await tick();
  }

  it('shows where each link sits in its page', async () => {
    await render();

    const cells = [...target.querySelectorAll('tbody td')].map((td) => td.textContent.trim());
    expect(cells).toContain('nav');
    expect(cells).toContain('/html/body/nav/ul/li[1]/a');
  });

  it('lines the filter inputs up with the columns they filter', async () => {
    await render();

    const headers = [...target.querySelectorAll('thead th')].map((th) => th.textContent.trim());
    const filters = [...target.querySelectorAll('thead input.filter-input')].map((i) =>
      i.getAttribute('placeholder'),
    );

    // The filter row is positional: each input sits under its own column.
    expect(filters).toEqual([
      'source_url',
      'target_url',
      'anchor_text',
      'tag',
      'landmark',
      'xpath',
    ]);
    expect(headers).toEqual(['Source', 'Target', 'Anchor Text', 'Tag', 'Landmark', 'XPath']);
  });

  it('exports the position alongside the link', async () => {
    await render();

    target.querySelector('.export-dropdown button').click();
    await tick();
    const csvButton = [...target.querySelectorAll('button')].find((b) =>
      /csv/i.test(b.textContent),
    );
    csvButton.click();
    await tick();
    await tick();

    expect(downloadCSV).toHaveBeenCalled();
    const [, headers, keys, rows] = downloadCSV.mock.calls[0];
    expect(headers).toEqual(
      expect.arrayContaining(['Landmark', 'XPath', 'Depth', 'Document Index', 'Block Signature']),
    );
    expect(keys).toEqual(
      expect.arrayContaining(['Landmark', 'XPath', 'Depth', 'DocumentIndex', 'BlockSignature']),
    );
    expect(rows[0].XPath).toBe('/html/body/nav/ul/li[1]/a');
  });
});
