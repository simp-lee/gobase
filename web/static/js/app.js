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

function toastManager() {
    return {
        toasts: [],
        addToast(detail) {
            const toast = {
                id: Date.now(),
                message: detail.message || 'Operation completed',
                type: detail.type || 'info',
                visible: true
            };
            this.toasts.push(toast);
            setTimeout(() => {
                this.removeToast(toast.id);
            }, 3000);
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
   When a server response includes HX-Trigger: {"showToast": {...}},
   parse the header and dispatch an Alpine-compatible custom event.
   -------------------------------------------------------------------------- */

document.addEventListener('htmx:afterRequest', function (evt) {
    var xhr = evt.detail.xhr;
    if (!xhr) return;

    var triggerHeader = xhr.getResponseHeader('HX-Trigger');
    if (triggerHeader) {
        try {
            var triggers = JSON.parse(triggerHeader);
            if (triggers.showToast) {
                window.dispatchEvent(new CustomEvent('show-toast', { detail: triggers.showToast }));
            }
        } catch (e) {
            // Not JSON – single-event trigger name, ignore
        }
    }
});

/* --------------------------------------------------------------------------
   3. htmx global event hooks
   Add any global htmx behaviour here (loading indicators, error handling…).
   -------------------------------------------------------------------------- */

// Example: log htmx swap errors to the console during development.
document.addEventListener('htmx:responseError', function (evt) {
    console.error('[htmx] Response error:', evt.detail.xhr.status, evt.detail.xhr.statusText);
});
