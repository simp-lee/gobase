/* ==========================================================================
   GoBase – app.js
   Global client-side logic:
     1. toastManager()  – Alpine.js component (M6)
     2. htmx HX-Trigger → Alpine.js bridge (M6)
     3. htmx global event hooks
   ========================================================================== */

/* --------------------------------------------------------------------------
   1. Alpine.js toast manager component (M6)
   Usage: <div x-data="toastManager()" @show-toast.window="addToast($event.detail)">
   -------------------------------------------------------------------------- */

function resolveToastDuration(detail) {
    if (detail && Number.isFinite(detail.duration) && detail.duration > 0) {
        return detail.duration;
    }

    return 5000;
}

window.__gobaseToastBus = window.__gobaseToastBus || {
    ready: false,
    queue: [],
    dispatch(detail) {
        const payload = detail || {};

        if (!this.ready) {
            this.queue.push(payload);
            return;
        }

        window.dispatchEvent(new CustomEvent('show-toast', { detail: payload }));
    },
    markReady() {
        if (this.ready) {
            return;
        }

        this.ready = true;
        while (this.queue.length > 0) {
            const payload = this.queue.shift();
            window.dispatchEvent(new CustomEvent('show-toast', { detail: payload }));
        }
    }
};

function toastManager() {
    return {
        nextToastId: 1,
        toasts: [],
        init() {
            window.__gobaseToastBus.markReady();
        },
        addToast(detail) {
            const toast = {
                id: this.nextToastId++,
                message: detail.message || 'Operation completed',
                type: detail.type || 'info',
                visible: true
            };
            this.toasts.push(toast);
            setTimeout(() => {
                this.removeToast(toast.id);
            }, resolveToastDuration(detail));
        },
        removeToast(id) {
            const toast = this.toasts.find(t => t.id === id);
            if (toast) {
                toast.visible = false;
                setTimeout(() => {
                    this.toasts = this.toasts.filter(t => t.id !== id);
                }, 300);
            }
        }
    };
}

/* --------------------------------------------------------------------------
   2. htmx HX-Trigger → Alpine.js bridge (M6)
   htmx already dispatches native custom events from HX-Trigger headers.
   Bridge the camelCase showToast event to Alpine's kebab-case listener once.
   -------------------------------------------------------------------------- */

document.body.addEventListener('showToast', function (evt) {
    window.__gobaseToastBus.dispatch(evt.detail || {});
});

/* --------------------------------------------------------------------------
   3. htmx global event hooks
   Add any global htmx behaviour here (loading indicators, error handling…).
   -------------------------------------------------------------------------- */

// Example: log htmx swap errors to the console during development.
document.addEventListener('htmx:responseError', function (evt) {
    console.error('[htmx] Response error:', evt.detail.xhr.status, evt.detail.xhr.statusText);
});
