async function fetchSecrets() {
    try {
        const response = await fetch('/api/secrets');
        if (!response.ok) {
            const errorData = await response.json().catch(() => ({ message: response.statusText }));
            throw new Error(`HTTP error! Status: ${response.status} - ${errorData.message || 'Unknown error'}`);
        }

        const secretsJson = await response.json();
        const secrets = secretsJson.secrets || [];
        const secretsList = document.querySelector('.secrets-list');
        secretsList.innerHTML = ''; // Clear existing items
        if (secrets && secrets.length > 0) {
            secrets.forEach(secret => {
                const secretItem = document.createElement('div');
                secretItem.className = 'secret-item';
                secretItem.innerHTML = `
                    <div class="secret-item-header">
                        <div class="secret-name">${secret.name}</div>
                    </div>
                    <div class="secret-preview"><span style="font-weight: bold">Namespace:</span> ${secret.namespace}</div>
                `;
                secretItem.addEventListener('click', function () {
                    const secretField = document.querySelectorAll('.input-field')[0];

                    const secretData = fetchSecretData(secret.namespace, secret.name);
                    secretData.then(data => {
                        if (data) {
                            console.log('Secret data:', data);
                            secretField.value = toYaml(data);
                        } else {
                            showSnackbar('Failed to fetch secret data.', 'error');
                        }
                    });

                    // Close the modal
                    document.getElementById('secrets-modal').classList.remove('active');
                });
                secretsList.appendChild(secretItem);
            });
        } else {
            const noSecretsMessage = document.createElement('div');
            noSecretsMessage.className = 'no-secrets';
            noSecretsMessage.innerHTML = `<span class="no-secrets-icon">🔒</span>No secrets found.`;
            secretsList.appendChild(noSecretsMessage);
        }
    } catch (error) {
        console.error('Error fetching secrets:', error);
    }
}

async function fetchSecretData(namespace, name) {
    try {
        const response = await fetch(`/api/secret/${namespace}/${name}`);
        if (!response.ok) {
            throw new Error(`HTTP error! Status: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('Error fetching secret data:', error);
    }
}

async function seal() {
    const secretField = document.querySelectorAll('.input-field')[0];
    const sealedSecretField = document.querySelectorAll('.input-field')[1];
    var secret = JSON.stringify(toJson(secretField.value));

    parsed = JSON.parse(secret);
    const firstKey = Object.keys(parsed.data)[0];
    const firstValue = parsed.data[firstKey];
    if (!isBase64(firstValue)) {
        secret = JSON.stringify(toggleSecretEncoding(parsed), null, 2);
    }

    try {
        console.log(secret);
        console.log(parsed);
        const response = await fetch('/api/kubeseal', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: secret
        });
        if (!response.ok) {
            throw new Error(`HTTP error! Status: ${response.status}`);
        }
        const data = await response.json();
        console.log(data);
        sealedSecretField.value = toYaml(data);
    } catch (error) {
        console.error('Error sealing secret:', error);
    }
}

function toggleSecretEncoding(secretJson) {
    const toggled = JSON.parse(JSON.stringify((secretJson))); // Deep copy

    Object.keys(toggled.data).forEach(key => {
        const value = toggled.data[key];

        // Try to decode - if it works, it was encoded
        try {
            const decoded = atob(value);
            // Check if the decoded value can be encoded back to the same value
            if (btoa(decoded) === value) {
                toggled.data[key] = decoded; // It was Base64, now decode it
            } else {
                toggled.data[key] = btoa(value); // It wasn't Base64, encode it
            }
            showSnackbar('Text successfully decoded', 'success');
        } catch (error) {
            // If decode fails, it's probably plain text, so encode it
            try {
                toggled.data[key] = btoa(value);
                showSnackbar('Text successfully encoded to Base64', 'success');
            } catch (encodeError) {
                console.log(`Error processing ${key}: ${encodeError.message}`);
            }
        }
    });

    return toggled;
}

function toggleEncodeDecode() {
    const text = toJson(secretTextarea.value.trim());
    if (!text) {
        showSnackbar('Please enter some text', 'error');
        return;
    }
    const toggled = toggleSecretEncoding(text);
    secretTextarea.value = toYaml(toggled);
}

function isBase64(str) {
    try {
        return btoa(atob(str)) === str;
    } catch (err) {
        return false;
    }
}

function toYaml(json) {
    return jsyaml.dump(json);
}

function toJson(yaml) {
    return jsyaml.load(yaml);
}