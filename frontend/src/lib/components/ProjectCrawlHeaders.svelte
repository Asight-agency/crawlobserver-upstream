<script>
  import { untrack } from 'svelte';
  import { setProjectCrawlHeaders } from '../api.js';
  import { t } from '../i18n/index.svelte.js';

  let { projectId, headers = {}, onerror, onsaved } = $props();

  /** Rows rather than a map, so that two half-typed names can coexist while
   * editing without one silently replacing the other. */
  function toRows(map) {
    const rows = Object.entries(map || {}).map(([name, value]) => ({ name, value }));
    return rows.length > 0 ? rows : [{ name: '', value: '' }];
  }

  // The rows are seeded from the project once and then owned by this
  // component: re-reading the prop would discard what is being typed the
  // moment the project is refreshed elsewhere.
  let rows = $state(untrack(() => toRows(headers)));
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');

  function addRow() {
    rows = [...rows, { name: '', value: '' }];
    saved = false;
  }

  function removeRow(index) {
    rows = rows.filter((_, i) => i !== index);
    if (rows.length === 0) rows = [{ name: '', value: '' }];
    saved = false;
  }

  function touched() {
    saved = false;
    error = '';
  }

  async function save() {
    if (saving) return;
    saving = true;
    error = '';

    const map = {};
    for (const row of rows) {
      const name = row.name.trim();
      if (name === '') continue;
      map[name] = row.value.trim();
    }

    try {
      await setProjectCrawlHeaders(projectId, map);
      rows = toRows(map);
      saved = true;
      onsaved?.(map);
    } catch (e) {
      // Shown here rather than only in the page banner: the message names the
      // header at fault, and it is read next to the field that holds it.
      error = e.message;
      onerror?.(e.message);
    } finally {
      saving = false;
    }
  }
</script>

<details class="crawl-headers">
  <summary>{t('project.crawlHeaders')}</summary>

  <p class="crawl-headers-desc">{t('project.crawlHeadersDesc')}</p>

  <div class="crawl-headers-rows">
    {#each rows as row, i (i)}
      <div class="crawl-headers-row">
        <input
          type="text"
          class="crawl-headers-name"
          placeholder={t('project.crawlHeaderName')}
          bind:value={row.name}
          oninput={touched}
          aria-label={t('project.crawlHeaderName')}
        />
        <input
          type="text"
          class="crawl-headers-value"
          placeholder={t('project.crawlHeaderValue')}
          bind:value={row.value}
          oninput={touched}
          aria-label={t('project.crawlHeaderValue')}
        />
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          onclick={() => removeRow(i)}
          title={t('common.delete')}
          aria-label={t('common.delete')}>×</button
        >
      </div>
    {/each}
  </div>

  {#if error}
    <p class="crawl-headers-error" role="alert">{error}</p>
  {/if}

  <div class="crawl-headers-actions">
    <button type="button" class="btn btn-sm" onclick={addRow}>{t('project.addCrawlHeader')}</button>
    <button type="button" class="btn btn-sm btn-primary" onclick={save} disabled={saving}>
      {saving ? t('common.saving') : t('common.save')}
    </button>
    {#if saved}
      <span class="crawl-headers-saved">{t('project.crawlHeadersSaved')}</span>
    {/if}
  </div>
</details>

<style>
  .crawl-headers {
    margin: 24px;
    border: 1px solid var(--border, #ddd);
    border-radius: 8px;
    padding: 12px 16px;
  }

  .crawl-headers summary {
    cursor: pointer;
    font-weight: 600;
  }

  .crawl-headers summary::-webkit-details-marker {
    color: var(--text-muted, #888);
  }

  .crawl-headers-desc {
    color: var(--text-muted, #666);
    font-size: 0.9em;
    margin: 12px 0;
  }

  .crawl-headers-rows {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .crawl-headers-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .crawl-headers-name {
    flex: 0 0 220px;
    min-width: 0;
  }

  .crawl-headers-value {
    flex: 1 1 auto;
    min-width: 0;
    font-family: var(--font-mono, monospace);
    font-size: 0.9em;
  }

  .crawl-headers-error {
    color: var(--danger, #c0392b);
    font-size: 0.9em;
    margin: 12px 0 0;
  }

  .crawl-headers-actions {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 12px;
  }

  .crawl-headers-saved {
    color: var(--text-muted, #666);
    font-size: 0.9em;
  }
</style>
