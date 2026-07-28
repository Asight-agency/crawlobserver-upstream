import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, tick, unmount } from 'svelte';
import SearchSelect from './SearchSelect.svelte';

describe('SearchSelect', () => {
  let component;
  let target;
  let originalScrollIntoView;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    originalScrollIntoView = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = vi.fn();
    vi.stubGlobal('requestAnimationFrame', (callback) => {
      callback();
      return 1;
    });
  });

  afterEach(async () => {
    if (component) await unmount(component);
    target.remove();
    component = null;
    if (originalScrollIntoView) Element.prototype.scrollIntoView = originalScrollIntoView;
    else delete Element.prototype.scrollIntoView;
    vi.unstubAllGlobals();
  });

  function render(props = {}) {
    component = mount(SearchSelect, {
      target,
      props: {
        id: 'test-select',
        options: [
          { value: 'one', label: 'One' },
          { value: 'two', label: 'Two' },
        ],
        value: 'one',
        ...props,
      },
    });
  }

  it('connects the combobox to its listbox with the required ARIA attributes', async () => {
    render();
    const trigger = target.querySelector('#test-select');

    expect(trigger.getAttribute('role')).toBe('combobox');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(trigger.getAttribute('aria-controls')).toBe('test-select-listbox');

    trigger.click();
    await tick();

    const listbox = target.querySelector('#test-select-listbox');
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(listbox.getAttribute('role')).toBe('listbox');
    expect(listbox.querySelectorAll('[role="option"]')).toHaveLength(2);
    expect(listbox.querySelector('[role="option"]').getAttribute('tabindex')).toBe('-1');
  });

  it('selects an option and returns focus to the trigger', async () => {
    const onchange = vi.fn();
    render({ onchange });
    const trigger = target.querySelector('#test-select');

    trigger.click();
    await tick();
    const secondOption = target.querySelectorAll('[role="option"]')[1];
    secondOption.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    await tick();

    expect(onchange).toHaveBeenCalledWith('two');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger);
  });

  it('supports opening and selecting with the keyboard', async () => {
    const onchange = vi.fn();
    render({ onchange });
    const trigger = target.querySelector('#test-select');

    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await tick();
    const search = target.querySelector('.ss-search');
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await tick();

    expect(onchange).toHaveBeenCalledWith('one');
    expect(target.querySelector('[role="listbox"]')).toBeNull();
  });
});
