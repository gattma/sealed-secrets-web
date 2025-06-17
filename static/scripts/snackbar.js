// --- Snackbar Logic ---
let snackbarContainer = null;
const activeSnackbars = new Set(); // To keep track of active snackbars

function ensureSnackbarContainer() {
    if (!snackbarContainer) {
        snackbarContainer = document.createElement('div');
        snackbarContainer.id = 'snackbar-container';
        document.body.appendChild(snackbarContainer);
    }
}

function showSnackbar(message, type = 'info', duration = 3000) {
    ensureSnackbarContainer();

    const snackbar = document.createElement('div');
    snackbar.className = `snackbar ${type}`; // Add base and type-specific class

    const messageSpan = document.createElement('span');
    messageSpan.className = 'message';
    messageSpan.textContent = message;
    snackbar.appendChild(messageSpan);

    const closeButton = document.createElement('button');
    closeButton.className = 'close-btn';
    closeButton.innerHTML = '×'; // '×' character
    closeButton.setAttribute('aria-label', 'Close notification');
    snackbar.appendChild(closeButton);

    // Add to DOM and active set
    snackbarContainer.appendChild(snackbar);
    activeSnackbars.add(snackbar);

    // Accessibility attributes
    snackbar.setAttribute('role', type === 'error' || type === 'warning' ? 'alert' : 'status');
    snackbar.setAttribute('aria-live', type === 'error' || type === 'warning' ? 'assertive' : 'polite');


    // Trigger the animation (slight delay to ensure transition works)
    requestAnimationFrame(() => {
        requestAnimationFrame(() => {
            snackbar.classList.add('show');
        });
    });

    const hide = () => {
        if (!activeSnackbars.has(snackbar)) return; // Already hidden

        snackbar.classList.remove('show');
        activeSnackbars.delete(snackbar);

        // Remove from DOM after animation
        snackbar.addEventListener('transitionend', () => {
            if (snackbar.parentNode) {
                snackbar.parentNode.removeChild(snackbar);
            }
        }, { once: true }); // Important: only fire once
    };

    // Auto-hide
    const timerId = setTimeout(hide, duration);

    // Manual close
    closeButton.addEventListener('click', () => {
        clearTimeout(timerId);
        hide();
    });
}