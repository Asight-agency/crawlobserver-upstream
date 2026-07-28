import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import ConfirmModal from './ConfirmModal.svelte';

describe('ConfirmModal', () => {
  let component;
  let target;
  let onconfirm;
  let oncancel;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    onconfirm = vi.fn();
    oncancel = vi.fn();
    component = mount(ConfirmModal, {
      target,
      props: {
        message: 'Delete this session?',
        confirmLabel: 'Delete',
        cancelLabel: 'Keep',
        onconfirm,
        oncancel,
      },
    });
  });

  afterEach(async () => {
    await unmount(component);
    target.remove();
  });

  it('exposes an accessible alert dialog', () => {
    const dialog = target.querySelector('[role="alertdialog"]');

    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('tabindex')).toBe('-1');
    expect(dialog.textContent).toContain('Delete this session?');
  });

  it('confirms from the primary action', () => {
    const buttons = target.querySelectorAll('button');
    buttons[1].click();

    expect(onconfirm).toHaveBeenCalledOnce();
    expect(oncancel).not.toHaveBeenCalled();
  });

  it('cancels from the overlay or Escape, but not from a click inside the dialog', () => {
    const overlay = target.querySelector('.confirm-overlay');
    const dialog = target.querySelector('.confirm-dialog');

    dialog.click();
    expect(oncancel).not.toHaveBeenCalled();

    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(oncancel).toHaveBeenCalledOnce();

    overlay.click();
    expect(oncancel).toHaveBeenCalledTimes(2);
  });
});
