// Global variable to store the base URL for API calls
let baseUrl = '';

// Initialize the application
document.addEventListener('DOMContentLoaded', function() {
    // Set the base URL from the current location
    baseUrl = window.location.origin;

    // Add click effects to vote buttons
    const voteButtons = document.querySelectorAll('.vote-btn');
    voteButtons.forEach(button => {
        button.addEventListener('click', function() {
            // Create a subtle click effect
            this.style.transform = 'scale(0.9)';
            setTimeout(() => {
                this.style.transform = '';
            }, 150);
        });
    });
});

// Function to handle voting asynchronously
async function vote(imageKey, voteType) {
    const voteElement = document.getElementById(`votes-${imageKey.replace(/[^a-zA-Z0-9]/g, '-')}`);
    const voteButtons = document.querySelectorAll(`[data-image-key="${imageKey}"] .vote-btn`);

    if (!voteElement) return;

    // Get current vote count
    const currentVotes = parseInt(voteElement.textContent) || 0;
    const voteIncrement = voteType === 'up' ? 1 : -1;

    // Optimistically update the UI immediately
    const newVoteCount = currentVotes + voteIncrement;
    voteElement.textContent = newVoteCount;

    // Add immediate visual feedback
    voteElement.style.transform = 'scale(1.2)';
    voteElement.style.color = voteType === 'up' ? '#4CAF50' : '#f44336';

    // Disable vote buttons to prevent double-clicking
    voteButtons.forEach(btn => {
        btn.disabled = true;
        btn.style.opacity = '0.6';
    });

    // Send vote request asynchronously
    const votePromise = fetch(`${baseUrl}/vote/${imageKey}/${voteType}`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        }
    });

    try {
        const response = await votePromise;

        if (response.ok) {
            const result = await response.json();

            // Update with actual server response
            voteElement.textContent = result.new_vote_count;

            // Success animation
            voteElement.style.transform = 'scale(1.1)';
            setTimeout(() => {
                voteElement.style.transform = 'scale(1)';
                voteElement.style.color = '';
            }, 300);

        } else {
            // Revert optimistic update on error
            voteElement.textContent = currentVotes;
            console.error('Vote failed:', response.statusText);

            // Show error feedback
            voteElement.style.transform = 'scale(1.1)';
            voteElement.style.color = '#f44336';
            setTimeout(() => {
                voteElement.style.transform = 'scale(1)';
                voteElement.style.color = '';
            }, 300);

            // Show user-friendly error message
            showNotification('Vote failed. Please try again.', 'error');
        }
    } catch (error) {
        // Revert optimistic update on network error
        voteElement.textContent = currentVotes;
        console.error('Error voting:', error);

        // Show error feedback
        voteElement.style.transform = 'scale(1.1)';
        voteElement.style.color = '#f44336';
        setTimeout(() => {
            voteElement.style.transform = 'scale(1)';
            voteElement.style.color = '';
        }, 300);

        // Show user-friendly error message
        showNotification('Network error. Please check your connection.', 'error');
    } finally {
        // Re-enable vote buttons
        voteButtons.forEach(btn => {
            btn.disabled = false;
            btn.style.opacity = '1';
        });
    }
}

// Function to show notifications
function showNotification(message, type = 'info') {
    // Remove existing notifications
    const existingNotification = document.querySelector('.notification');
    if (existingNotification) {
        existingNotification.remove();
    }

    // Create notification element
    const notification = document.createElement('div');
    notification.className = `notification notification-${type}`;
    notification.textContent = message;

    // Add styles
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 12px 20px;
        border-radius: 8px;
        color: white;
        font-weight: 500;
        z-index: 1000;
        transform: translateX(100%);
        transition: transform 0.3s ease;
        max-width: 300px;
        word-wrap: break-word;
    `;

    // Set background color based on type
    if (type === 'error') {
        notification.style.backgroundColor = '#f44336';
    } else if (type === 'success') {
        notification.style.backgroundColor = '#4CAF50';
    } else {
        notification.style.backgroundColor = '#2196F3';
    }

    // Add to page
    document.body.appendChild(notification);

    // Animate in
    setTimeout(() => {
        notification.style.transform = 'translateX(0)';
    }, 100);

    // Auto-remove after 3 seconds
    setTimeout(() => {
        notification.style.transform = 'translateX(100%)';
        setTimeout(() => {
            if (notification.parentNode) {
                notification.remove();
            }
        }, 300);
    }, 3000);
}
