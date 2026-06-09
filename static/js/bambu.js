// FilaBridge - Bambu Lab (Home Assistant) integration UI

function bambuFormatDuration(seconds) {
    if (seconds == null || seconds < 0) return '--';
    const total = Math.round(seconds);
    if (total <= 0) return '0s';
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = total % 60;
    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m ${secs}s`;
    return `${secs}s`;
}

function bambuProgressPercent(progress) {
    if (progress == null) return 0;
    const pct = progress <= 1 ? progress * 100 : progress;
    return Math.min(100, Math.max(0, pct));
}

function bambuSpoolLabel(spool) {
    const material = spool.material || 'Unknown Material';
    const brand = spool.brand || 'Unknown Brand';
    const name = spool.name || 'Unnamed Spool';
    const weight = spool.remaining_weight != null ? ` (${Math.round(spool.remaining_weight)}g remaining)` : '';
    return `[${spool.id}] ${material} - ${brand} - ${name}${weight}`;
}

function bambuSpoolColorHex(spool) {
    return (spool.filament && spool.filament.color_hex) ? spool.filament.color_hex.replace('#', '') : 'ccc';
}

function bambuStatusClass(state) {
    return (state || 'idle').toLowerCase();
}

function collectBambuTrays(printer) {
    const allTrays = [];
    (printer.external_spools || []).forEach(t => {
        allTrays.push({ ...t, label: t.display_name || 'External Spool' });
    });
    (printer.ams_units || []).forEach(ams => {
        (ams.trays || []).forEach(t => {
            allTrays.push({ ...t, label: t.display_name || `${ams.name} Slot ${t.tray_number}` });
        });
    });
    return allTrays;
}

function findBambuSpoolById(spools, spoolId) {
    if (spoolId == null || spoolId === '') return null;
    return spools.find(s => String(s.id) === String(spoolId)) || null;
}

function buildBambuTrayButtonHTML(spool) {
    if (!spool) {
        return `
            <div style="display: flex; align-items: center; gap: 10px;">
                <div class="color-swatch" style="background-color: #ccc;"></div>
                <span>Empty</span>
            </div>`;
    }
    const color = bambuSpoolColorHex(spool);
    return `
        <div style="display: flex; align-items: center; gap: 10px;">
            <div class="color-swatch" data-color="${color}"></div>
            <span>${bambuSpoolLabel(spool)}</span>
        </div>`;
}

function buildBambuTrayDropdownHTML(tray, spools) {
    const assignedId = tray.assigned_spool_id ?? '';
    let buttonInner = buildBambuTrayButtonHTML(null);
    let editHidden = 'hidden';
    let editSpoolId = '';
    let editColorHex = '';

    if (assignedId) {
        const spool = findBambuSpoolById(spools, assignedId);
        if (spool) {
            buttonInner = buildBambuTrayButtonHTML(spool);
            editHidden = '';
            editSpoolId = String(spool.id);
            editColorHex = bambuSpoolColorHex(spool);
        }
    }

    return `
        <div class="bambu-tray-mapping-row" style="display: flex; align-items: center; gap: 15px; margin-bottom: 10px; padding: 10px; background: rgba(255,255,255,0.05); border-radius: 5px;" data-tray-unique-id="${tray.unique_id}">
            <div class="toolhead-label" style="min-width: 100px; font-weight: bold;">${tray.label}:</div>
            <div class="custom-dropdown bambu-tray-dropdown" style="flex: 1;">
                <div class="dropdown-button">
                    ${buttonInner}
                    <span class="dropdown-arrow">▼</span>
                </div>
                <div class="dropdown-content">
                    <div class="dropdown-search-container">
                        <input type="text" class="dropdown-search" placeholder="Search spools..." autocomplete="off">
                    </div>
                    <div class="dropdown-options-container">
                        <div class="dropdown-option" data-value="" data-color="">
                            <div class="color-swatch" style="background-color: #ccc;"></div>
                            <div class="option-text">Empty</div>
                        </div>
                        <div class="dropdown-no-results">No spools found</div>
                    </div>
                </div>
                <input type="hidden" value="${assignedId}">
            </div>
            <button class="edit-spool-btn ${editHidden}"
                    data-spool-id="${editSpoolId}"
                    data-color-hex="${editColorHex}"
                    onclick="openSpoolmanEdit(this.dataset.spoolId)">
                ✏️ Edit
            </button>
        </div>
    `;
}

function renderBambuPrinterCard(printer, spools) {
    const card = document.createElement('div');
    card.className = 'printer bambu-printer';
    card.dataset.printerId = printer.printer_id || '';

    const state = printer.state || 'IDLE';
    const isPrinting = state === 'PRINTING';
    const progress = bambuProgressPercent(printer.progress);
    const allTrays = collectBambuTrays(printer);

    let traysHTML = '';
    for (const tray of allTrays) {
        traysHTML += buildBambuTrayDropdownHTML(tray, spools);
    }

    const layerVisible = printer.current_layer != null && printer.total_layer != null;
    const layerText = layerVisible ? `Layer ${printer.current_layer} / ${printer.total_layer}` : '';

    card.innerHTML = `
        <div class="printer-header">
            <h3>${printer.name}</h3>
            <span class="status ${bambuStatusClass(state)}">${state}</span>
        </div>
        <p><strong>Model:</strong> Bambu Lab (Home Assistant)</p>
        <div class="print-job-section"${isPrinting ? '' : ' style="display: none;"'}>
            <p class="print-job-name"><strong>Printing:</strong> <span class="job-name">${printer.job_name || ''}</span></p>
            <div class="print-progress">
                <div class="print-progress-bar" style="width: ${progress.toFixed(1)}%;"></div>
            </div>
            <p class="print-timing">
                <span class="progress-label">${progress.toFixed(1)}%</span>
                · <span class="print-duration">Elapsed: ${bambuFormatDuration(printer.print_duration)}</span>
                · <span class="time-remaining">Remaining: ${bambuFormatDuration(printer.time_remaining)}</span>
                · <span class="layer-info"${layerVisible ? '' : ' style="display: none;"'}>${layerText}</span>
            </p>
        </div>
        <div class="mapping-section">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                <h3>AMS Tray Mappings</h3>
            </div>
            <h4>Map Spools to Trays</h4>
            <div class="bambu-trays-container" style="margin-top: 15px;">
                ${traysHTML || '<p>No trays discovered</p>'}
            </div>
        </div>
    `;

    initBambuTrayDropdowns(card);
    if (typeof initColorSwatches === 'function') initColorSwatches();
    if (typeof initEditButtonColors === 'function') initEditButtonColors();

    return card;
}

async function loadAvailableBambuSpools(dropdown) {
    const row = dropdown.closest('.bambu-tray-mapping-row');
    if (!row) return;

    const trayUniqueId = row.dataset.trayUniqueId;
    const hiddenInput = dropdown.querySelector('input[type="hidden"]');
    const currentSpoolId = hiddenInput ? hiddenInput.value : '';

    try {
        const response = await fetch(`/api/available_spools?tray_unique_id=${encodeURIComponent(trayUniqueId)}`);
        const data = await response.json();
        if (data.error) {
            console.error('Error loading available spools for Bambu tray:', data.error);
            return;
        }

        const optionsContainer = dropdown.querySelector('.dropdown-options-container');
        if (!optionsContainer) return;

        const emptyOption = optionsContainer.querySelector('.dropdown-option[data-value=""]');
        optionsContainer.innerHTML = '';
        if (emptyOption) {
            optionsContainer.appendChild(emptyOption);
        } else {
            const empty = document.createElement('div');
            empty.className = 'dropdown-option';
            empty.dataset.value = '';
            empty.dataset.color = '';
            empty.innerHTML = '<div class="color-swatch" style="background-color: #ccc;"></div><div class="option-text">Empty</div>';
            optionsContainer.appendChild(empty);
        }

        const optionSpools = [...(data.spools || [])];
        if (currentSpoolId && !optionSpools.some(s => String(s.id) === String(currentSpoolId))) {
            try {
                const allRes = await fetch('/api/spools');
                if (allRes.ok) {
                    const allSpools = await allRes.json();
                    const current = findBambuSpoolById(allSpools, currentSpoolId);
                    if (current) optionSpools.unshift(current);
                }
            } catch (_) { /* keep available list only */ }
        }

        optionSpools.forEach(spool => {
            const option = document.createElement('div');
            option.className = 'dropdown-option';
            if (currentSpoolId && String(spool.id) === String(currentSpoolId)) {
                option.classList.add('selected');
            }
            const color = bambuSpoolColorHex(spool);
            option.dataset.value = spool.id;
            option.dataset.color = color;
            option.innerHTML = `
                <div class="color-swatch" data-color="${color}"></div>
                <div class="option-text">${bambuSpoolLabel(spool)}</div>
            `;
            optionsContainer.appendChild(option);
        });

        const noResults = document.createElement('div');
        noResults.className = 'dropdown-no-results';
        noResults.textContent = 'No spools found';
        optionsContainer.appendChild(noResults);

        bindBambuDropdownOptions(dropdown);
    } catch (error) {
        console.error('Error loading available Bambu spools:', error);
    }
}

async function assignBambuTraySpool(dropdown, selectedValue, selectedText, selectedColor) {
    const row = dropdown.closest('.bambu-tray-mapping-row');
    if (!row) return;

    const trayUniqueId = row.dataset.trayUniqueId;
    const button = dropdown.querySelector('.dropdown-button');
    const originalContent = button.innerHTML;

    button.innerHTML = `
        <div style="display: flex; align-items: center; gap: 10px;">
            <div class="color-swatch" style="background-color: #${selectedColor || 'ccc'};"></div>
            <span>${selectedText}</span>
        </div>
        <span class="dropdown-arrow">⏳</span>
    `;

    try {
        const spoolId = selectedValue && selectedValue !== '' ? parseInt(selectedValue, 10) : 0;
        const res = await fetch('/api/trays/assign', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ spool_id: spoolId, tray_unique_id: trayUniqueId })
        });
        const data = await res.json();
        if (!res.ok) {
            throw new Error(data.error || res.statusText);
        }

        const hiddenInput = dropdown.querySelector('input[type="hidden"]');
        if (hiddenInput) hiddenInput.value = selectedValue || '';

        button.innerHTML = `
            <div style="display: flex; align-items: center; gap: 10px;">
                <div class="color-swatch" style="background-color: #${selectedColor || 'ccc'};"></div>
                <span>${selectedText}</span>
            </div>
            <span class="dropdown-arrow">✅</span>
        `;

        updateEditButton(row, selectedValue, selectedColor);

        setTimeout(() => {
            button.innerHTML = `
                <div style="display: flex; align-items: center; gap: 10px;">
                    <div class="color-swatch" style="background-color: #${selectedColor || 'ccc'};"></div>
                    <span>${selectedText}</span>
                </div>
                <span class="dropdown-arrow">▼</span>
            `;
        }, 2000);

        syncBambuCardTraySignature(row.closest('[data-printer-id]'));

        if (typeof refreshAllDropdowns === 'function') await refreshAllDropdowns();
    } catch (error) {
        alert('Assign failed: ' + error.message);
        button.innerHTML = originalContent;
    }
}

function bindBambuDropdownOptions(dropdown) {
    const content = dropdown.querySelector('.dropdown-content');
    const optionsContainer = dropdown.querySelector('.dropdown-options-container');
    if (!content || !optionsContainer) return;

    const existingOptions = [...optionsContainer.querySelectorAll('.dropdown-option')];
    for (const option of existingOptions) {
        option.replaceWith(option.cloneNode(true));
    }

    optionsContainer.querySelectorAll('.dropdown-option').forEach(option => {
        option.addEventListener('click', async (e) => {
            e.stopPropagation();

            const selectedText = option.querySelector('.option-text').textContent;
            const selectedColor = option.dataset.color;
            const selectedValue = option.dataset.value;

            const hiddenInput = dropdown.querySelector('input[type="hidden"]');
            if (hiddenInput) hiddenInput.value = selectedValue;

            optionsContainer.querySelectorAll('.dropdown-option').forEach(opt => opt.classList.remove('selected'));
            option.classList.add('selected');

            content.classList.remove('show');
            dropdown.querySelector('.dropdown-button').classList.remove('open');
            dropdown.querySelector('.dropdown-arrow').classList.remove('open');

            await assignBambuTraySpool(dropdown, selectedValue, selectedText, selectedColor);
        });
    });
}

function initBambuTrayDropdowns(root) {
    const scope = root || document;
    scope.querySelectorAll('.bambu-tray-dropdown').forEach(dropdown => {
        if (dropdown.dataset.bambuInit === '1') return;
        dropdown.dataset.bambuInit = '1';

        const button = dropdown.querySelector('.dropdown-button');
        const content = dropdown.querySelector('.dropdown-content');
        const arrow = dropdown.querySelector('.dropdown-arrow');
        const searchInput = dropdown.querySelector('.dropdown-search');
        const optionsContainer = dropdown.querySelector('.dropdown-options-container');
        const noResults = dropdown.querySelector('.dropdown-no-results');

        if (searchInput) {
            searchInput.addEventListener('input', (e) => {
                const searchTerm = e.target.value.toLowerCase().trim();
                const options = optionsContainer.querySelectorAll('.dropdown-option');
                let visibleCount = 0;

                options.forEach(option => {
                    const optionText = option.querySelector('.option-text').textContent.toLowerCase();
                    let isMatch = searchTerm === '';
                    if (searchTerm !== '') {
                        if (/^\d+$/.test(searchTerm)) {
                            const idMatch = optionText.match(/^\[(\d+)\]/);
                            isMatch = idMatch && idMatch[1] === searchTerm;
                        } else {
                            const searchRegex = new RegExp('\\b' + searchTerm.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i');
                            isMatch = searchRegex.test(optionText);
                        }
                    }
                    option.style.display = isMatch ? 'flex' : 'none';
                    if (isMatch) visibleCount++;
                });

                if (noResults) {
                    noResults.style.display = (visibleCount === 0 && searchTerm !== '') ? 'block' : 'none';
                }
            });
        }

        bindBambuDropdownOptions(dropdown);

        button.addEventListener('click', async (e) => {
            e.stopPropagation();

            document.querySelectorAll('.custom-dropdown').forEach(other => {
                other.querySelector('.dropdown-content')?.classList.remove('show');
                other.querySelector('.dropdown-button')?.classList.remove('open');
                other.querySelector('.dropdown-arrow')?.classList.remove('open');
                const otherSearch = other.querySelector('.dropdown-search');
                if (otherSearch) {
                    otherSearch.value = '';
                    otherSearch.dispatchEvent(new Event('input'));
                }
            });

            const isOpening = !content.classList.contains('show');
            content.classList.toggle('show');
            button.classList.toggle('open');
            arrow.classList.toggle('open');

            if (isOpening) {
                await loadAvailableBambuSpools(dropdown);
                if (searchInput) {
                    setTimeout(() => searchInput.focus(), 10);
                }
            }
        });
    });
}

function syncBambuCardTraySignature(card) {
    if (!card) return;
    const parts = [...card.querySelectorAll('.bambu-tray-mapping-row')].map(row => {
        const uid = row.dataset.trayUniqueId || '';
        const hidden = row.querySelector('input[type="hidden"]');
        return `${uid}:${hidden?.value || ''}`;
    });
    card.dataset.traySignature = parts.join('|');
}

function updateBambuTrayButtonFromSpools(dropdown, spools) {
    const hiddenInput = dropdown.querySelector('input[type="hidden"]');
    const button = dropdown.querySelector('.dropdown-button');
    if (!hiddenInput || !button) return;

    const spool = findBambuSpoolById(spools, hiddenInput.value);
    const row = dropdown.closest('.bambu-tray-mapping-row');
    button.innerHTML = `${buildBambuTrayButtonHTML(spool)}<span class="dropdown-arrow">▼</span>`;
    if (row && typeof updateEditButton === 'function') {
        updateEditButton(row, hiddenInput.value || '', spool ? bambuSpoolColorHex(spool) : '');
    }
}

function updateBambuTrayMappings(card, printer, spools) {
    const container = card.querySelector('.bambu-trays-container');
    if (!container) return;

    const trays = collectBambuTrays(printer);
    const signature = trays.map(t => `${t.unique_id}:${t.assigned_spool_id ?? ''}`).join('|');
    if (card.dataset.traySignature === signature) {
        card.querySelectorAll('.bambu-tray-dropdown').forEach(dropdown => {
            updateBambuTrayButtonFromSpools(dropdown, spools);
        });
        if (typeof initColorSwatches === 'function') initColorSwatches();
        if (typeof initEditButtonColors === 'function') initEditButtonColors();
        return;
    }

    card.dataset.traySignature = signature;
    let traysHTML = '';
    for (const tray of trays) {
        traysHTML += buildBambuTrayDropdownHTML(tray, spools);
    }
    container.innerHTML = traysHTML || '<p>No trays discovered</p>';
    initBambuTrayDropdowns(card);
    if (typeof initColorSwatches === 'function') initColorSwatches();
    if (typeof initEditButtonColors === 'function') initEditButtonColors();
}

async function loadBambuPrintersStatus(forceRebuild = false) {
    const container = document.getElementById('bambu-printers-container');
    if (!container) return;

    try {
        const response = await fetch('/api/ha/printers');
        if (!response.ok) {
            const err = await response.json();
            container.innerHTML = `<p class="help-text">Bambu printers: ${err.error || 'configure Home Assistant in Settings'}</p>`;
            return;
        }
        const printers = await response.json();
        const registered = printers.filter(p => p.registered);
        if (registered.length === 0) {
            container.innerHTML = '<p class="help-text">No Bambu Lab printers registered. Add one in Settings → Printers.</p>';
            return;
        }

        const spoolsResponse = await fetch('/api/spools');
        const spools = spoolsResponse.ok ? await spoolsResponse.json() : [];

        if (forceRebuild || container.children.length === 0 || container.querySelector('.help-text')) {
            container.innerHTML = '';
            for (const printer of registered) {
                const card = renderBambuPrinterCard(printer, spools);
                syncBambuCardTraySignature(card);
                container.appendChild(card);
            }
            return;
        }

        for (const printer of registered) {
            const existing = container.querySelector(`[data-printer-id="${printer.printer_id}"]`);
            if (existing) {
                updateBambuTrayMappings(existing, printer, spools);
                if (typeof updatePrinterStatuses === 'function') {
                    updatePrinterStatuses({ [printer.printer_id]: {
                        name: printer.name,
                        state: printer.state,
                        job_name: printer.job_name,
                        progress: printer.progress,
                        print_duration: printer.print_duration,
                        time_remaining: printer.time_remaining,
                        current_layer: printer.current_layer,
                        total_layer: printer.total_layer
                    }});
                }
            } else {
                const card = renderBambuPrinterCard(printer, spools);
                syncBambuCardTraySignature(card);
                container.appendChild(card);
            }
        }

        container.querySelectorAll('[data-printer-id]').forEach(el => {
            const id = el.dataset.printerId;
            if (!registered.some(p => p.printer_id === id)) {
                el.remove();
            }
        });
    } catch (error) {
        console.error('Error loading Bambu printers:', error);
        container.innerHTML = '<p class="help-text">Failed to load Bambu printers.</p>';
    }
}

async function downloadHAConfig(printerId) {
    try {
        const res = await fetch(`/api/ha/automations/${printerId}`);
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Failed to generate config');
        const blob = new Blob([data.yaml], { type: 'text/yaml' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = data.filename || 'filabridge_ha.yaml';
        a.click();
        URL.revokeObjectURL(a.href);
        alert('YAML downloaded. Copy to HA config/packages/ and restart Home Assistant.\nWebhook URL: ' + data.webhook_url);
    } catch (e) {
        alert('Error: ' + e.message);
    }
}

async function loadHASettings() {
    const container = document.getElementById('ha-config-section');
    if (!container) return;
    try {
        const res = await fetch('/api/ha/config');
        const cfg = await res.json();
        container.innerHTML = `
            <h3>🏠 Home Assistant (Bambu Lab)</h3>
            <p class="help-text">Connect to Home Assistant with ha-bambulab integration for automatic filament tracking.</p>
            <div class="form-group">
                <label>HA URL</label>
                <input type="text" id="ha_url" value="${cfg.ha_url || ''}" placeholder="http://192.168.1.10:8123">
            </div>
            <div class="form-group">
                <label>HA Token ${cfg.ha_token_set ? '(configured)' : '(not set)'}</label>
                <input type="password" id="ha_token" placeholder="Long-Lived Access Token">
            </div>
            <div class="form-group">
                <label>FilaBridge Public URL (for HA webhooks)</label>
                <input type="text" id="filabridge_public_url" value="${cfg.filabridge_public_url || ''}" placeholder="http://192.168.1.20:5000">
                <small>Must be reachable from Home Assistant (not localhost if HA is on another machine)</small>
            </div>
            <div style="display:flex;gap:10px;margin-top:15px;">
                <button class="btn" onclick="saveHASettings()">💾 Save HA Settings</button>
                <button class="btn btn-secondary" onclick="testHAConnection()">🔌 Test Connection</button>
            </div>
            <p style="margin-top:10px;"><small>Setup guide: <code>docs/home-assistant-setup.md</code> in the FilaBridge repository</small></p>
        `;
    } catch (e) {
        container.innerHTML = '<p>Failed to load HA settings</p>';
    }
}

async function saveHASettings() {
    const body = {
        ha_url: document.getElementById('ha_url').value.trim(),
        filabridge_public_url: document.getElementById('filabridge_public_url').value.trim()
    };
    const token = document.getElementById('ha_token').value.trim();
    if (token) body.ha_token = token;
    if (!body.ha_url) {
        alert('Please enter the Home Assistant URL');
        return;
    }
    const res = await fetch('/api/ha/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    const data = await res.json();
    if (!res.ok) alert('Error: ' + (data.error || res.statusText));
    else alert('HA settings saved');
}

async function testHAConnection() {
    const haUrlEl = document.getElementById('ha_url');
    const haTokenEl = document.getElementById('ha_token');
    if (!haUrlEl) {
        alert('HA settings form not loaded');
        return;
    }
    const body = { ha_url: haUrlEl.value.trim() };
    if (haTokenEl && haTokenEl.value.trim()) {
        body.ha_token = haTokenEl.value.trim();
    }
    if (!body.ha_url) {
        alert('Please enter the Home Assistant URL (e.g. http://192.168.1.10:8123)');
        return;
    }
    const res = await fetch('/api/ha/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    const data = await res.json();
    alert(data.success ? 'Home Assistant connection OK!' : ('Connection failed: ' + (data.error || 'unknown')));
}

async function showAddBambuPrinterModal() {
    document.getElementById('addBambuPrinterModal').style.display = 'block';
    const list = document.getElementById('bambu-discover-list');
    list.innerHTML = '<p>Discovering printers...</p>';
    try {
        const res = await fetch('/api/ha/printers');
        const printers = await res.json();
        if (!res.ok) throw new Error(printers.error || 'Discovery failed');
        const available = printers.filter(p => !p.registered);
        if (available.length === 0) {
            list.innerHTML = '<p>No unregistered Bambu printers found in Home Assistant.</p>';
            return;
        }
        list.innerHTML = '';
        available.forEach(p => {
            const btn = document.createElement('button');
            btn.className = 'btn';
            btn.style.margin = '5px';
            btn.textContent = `➕ ${p.name} (${p.prefix})`;
            btn.onclick = () => registerBambuPrinter(p);
            list.appendChild(btn);
        });
    } catch (e) {
        list.innerHTML = `<p>Error: ${e.message}</p>`;
    }
}

function closeAddBambuPrinterModal() {
    document.getElementById('addBambuPrinterModal').style.display = 'none';
}

async function registerBambuPrinter(printer) {
    const res = await fetch('/api/ha/printers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(printer)
    });
    const data = await res.json();
    if (!res.ok) {
        alert('Error: ' + (data.error || res.statusText));
        return;
    }
    closeAddBambuPrinterModal();
    loadPrinters();
    loadBambuPrintersStatus(true);
    alert('Bambu printer registered! Generate HA config and restart Home Assistant.');
}

document.addEventListener('DOMContentLoaded', () => {
    loadBambuPrintersStatus(true);
    setInterval(() => loadBambuPrintersStatus(false), 30000);
});
