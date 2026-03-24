'use strict';
'require view';
'require dom';
'require poll';
'require rpc';
'require ui';

var callVoltStatus = rpc.declare({
	object: 'luci.zh_volt',
	method: 'status',
	expect: { data: [] }
});

var callVoltOnuActivate = rpc.declare({
	object: 'luci.zh_volt',
	method: 'onu_activate',
	expect: { result: false }
});

var callVoltOnuDeactivate = rpc.declare({
	object: 'luci.zh_volt',
	method: 'onu_deactivate',
	expect: { result: false }
});

function format(msg, ...args) {
	var i = 0;
	return msg.replace(/%[sdf]/g, function (match) {
		return typeof args[i] !== 'undefined' ? args[i++] : match;
	});
}

function timeoutPromise(promise, timeoutMs) {
	let timeoutID
	const timeout = new Promise((_, reject) => {
		timeoutID = setTimeout(() => {
			clearTimeout(timeoutID);
			reject(new Error('Operation timed out'));
		}, timeoutMs);
	});

	return Promise.race([promise, timeout])
		.finally(() => {
			clearTimeout(timeoutID);
		});
}

// Helper function to update text only if it is different (avoids unnecessary reflow)
function updateText(id, value) {
	var el = document.getElementById(id);
	if (el && el.textContent !== String(value)) {
		el.textContent = value;
	}
}

// Function to handle button clicks (avoids recreating the logic in each loop)
function handleAction(ev, action, mac, id) {
	var btn = ev.target;
	var originalText = btn.textContent;
	btn.disabled = true;
	btn.textContent = _('Wait...');
	btn.classList.add('spinning');

	var rpcCall = (action === 'activate') ? callVoltOnuActivate : callVoltOnuDeactivate;
	var msg = (action === 'activate') ? _('Activating...') : _('Deactivating...');

	ui.addNotification(null, E('p', format('%s (ONU %d @ %s)', msg, id, mac)), 'notice');
	timeoutPromise(rpcCall(mac, id), 40).then(function (res) {
		console.log(res);
		btn.disabled = false;
		btn.textContent = originalText;
		btn.classList.remove('spinning');
		if (res === true) {
			ui.addTimeLimitedNotification(null, E('p', _('Operation successful.')), 5000, 'info');
		} else {
			ui.addTimeLimitedNotification(null, E('p', _('Operation failed.')), 5000, 'danger');
		}
	}).catch(function (err) {
		btn.disabled = false;
		btn.textContent = originalText;
		btn.classList.remove('spinning');
		ui.addNotification(_('Error'), E('p', err.message), 'danger');
	});
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	render: function () {
		var container = E('div', { 'class': 'cbi-map' }, [
			E('h2', { 'name': 'content' }, _('OLT Status (zh-volt)')),
			E('div', { 'class': 'cbi-map-descr' }, _('Overview of the status of the OLT and ONUs connected to the network.'))
		]);

		var oltsContainer = E('div', { 'id': 'olts-container' }, [
			E('div', { 'id': 'loading-status', 'class': 'spinning' }, _('Loading data from OLT...'))
		]);

		container.appendChild(oltsContainer);

		poll.add(async function () {
			try {
				/** @type {({uptime: string, mac_addr: string, firmware_version: string, olt_dna: string, temperature: number, max_temperature: number, omci_mode: number, omci_error: number, online_onu: number, max_onu: number, onus: {id: number, status: string, uptime: string, sn: string, voltage: number, current: number, tx_power: number, rx_power: number, temperature: number}[]}[])} */
				const olts = await callVoltStatus();
				var contentNode = document.getElementById('olts-container');
				if (!contentNode)
					return;

				var loader = document.getElementById('loading-status');
				if (loader)
					loader.remove();

				if (!olts || olts.length === 0) {
					dom.content(contentNode, E('div', { 'class': 'alert-message warning' }, _('No OLT discovered yet.')));
					return;
				}

				for (const olt of olts) {
					var oltId = olt.mac_addr.replace(/:/g, '');
					var sectionId = 'olt-section-' + oltId;

					// If the OLT section does not exist, create the basic structure
					var section = document.getElementById(sectionId);
					if (!section) {
						section = E('fieldset', { 'id': sectionId, 'class': 'cbi-section' }, [
							E('h3', format(_('OLT information: %s'), olt.mac_addr)),
							E('div', { 'class': 'cbi-section-node' }, [
								E('table', { 'class': 'table' }, [
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', _('Uptime') + ":")), E('td', { 'id': 'olt-uptime-' + oltId, 'class': 'td left' }, olt.uptime)]),
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', _('Firmware Version') + ":")), E('td', { 'id': 'olt-firmware-' + oltId, 'class': 'td left' }, olt.firmware_version)]),
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', _('DNA OLT') + ":")), E('td', { 'id': 'olt-dna-' + oltId, 'class': 'td left' }, olt.olt_dna)]),
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', _('Temperature') + ":")), E('td', { 'id': 'olt-temp-' + oltId, 'class': 'td left' }, '')]),
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', _('Max Temperature') + ":")), E('td', { 'id': 'olt-max-temp-' + oltId, 'class': 'td left' }, '')]),
									E('tr', { 'class': 'tr' }, [E('td', { 'class': 'td left' }, E('strong', 'ONUs:')), E('td', { 'id': 'olt-onus-count-' + oltId, 'class': 'td left' }, '')])
								])
							]),
							E('br'),
							E('h3', _('ONU\'s status')),
							E('div', { 'class': 'cbi-section-node' }, [
								E('table', { 'id': 'onu-table-' + oltId, 'class': 'table w100' }, [
									E('tr', { 'class': 'tr table-titles' }, [
										E('th', { 'class': 'th' }, _('ID')),
										E('th', { 'class': 'th' }, _('Status')),
										E('th', { 'class': 'th' }, _('GPON Serial Number (SN)')),
										E('th', { 'class': 'th' }, _('Uptime')),
										E('th', { 'class': 'th' }, _('Temperature (°C)')),
										E('th', { 'class': 'th' }, _('RX/TX Power')),
										E('th', { 'class': 'th' }, _('Actions')),
									])
								])
							])
						]);
						contentNode.appendChild(section);
					}

					// Update OLT data
					updateText('olt-uptime-' + oltId, olt.uptime);
					updateText('olt-firmware-' + oltId, olt.firmware_version);
					updateText('olt-dna-' + oltId, olt.olt_dna);
					updateText('olt-temp-' + oltId, format(_('%s °C'), String(olt.temperature)));
					updateText('olt-max-temp-' + oltId, format(_('%s °C'), String(olt.max_temperature)));
					updateText('olt-onus-count-' + oltId, format(_('Online: %d / Max: %d'), String(olt.online_onu), String(olt.max_onu)));

					// Update ONUs
					var onuTable = document.getElementById('onu-table-' + oltId);
					for (const onu of olt.onus) {
						var rowId = 'onu-row-' + oltId + '-' + onu.id;
						var row = document.getElementById(rowId);

						var statusText = onu.status[0].toUpperCase() + onu.status.slice(1);
						var rxTxText = (onu.status === 'online') ? format(_("%d / %d"), String(onu.rx_power), String(onu.tx_power)) : '-';

						// Color Logic
						var statusColor = '#9E9E9E';
						if (onu.status === 'online') statusColor = '#4CAF50';
						else if (onu.status === 'disconnected') statusColor = '#F44336';
						else if (onu.status === 'omci') statusColor = '#FF9800';

						if (!row) {
							// Create the line if it doesn't exist
							row = E('tr', { 'id': rowId, 'class': 'tr' }, [
								E('td', { 'class': 'td', 'data-title': _('ID') }, String(onu.id)),
								E('td', { 'id': rowId + '-status', 'class': 'td', 'data-title': _('Status'), 'style': 'font-weight:bold; color:' + statusColor }, statusText),
								E('td', { 'id': rowId + '-sn', 'class': 'td', 'data-title': _('GPON SN') }, onu.sn === '0000-00000000' ? '-' : onu.sn),
								E('td', { 'id': rowId + '-uptime', 'class': 'td', 'data-title': _('Uptime') }, onu.uptime === '0s' ? '-' : onu.uptime),
								E('td', { 'id': rowId + '-temp', 'class': 'td', 'data-title': _('Temperature') }, onu.temperature === 0 ? '-' : String(onu.temperature)),
								E('td', { 'id': rowId + '-power', 'class': 'td', 'data-title': _('RX/TX Power') }, rxTxText),
								E('td', { 'class': 'td', 'data-title': _('Actions') }, [
									E('button', {
										'class': 'cbi-button cbi-button-action',
										'click': function (ev) { handleAction(ev, 'activate', olt.mac_addr, onu.id); }
									}, _('Enable')),
									E('span', { 'style': 'margin-right: 5px;' }),
									E('button', {
										'class': 'cbi-button cbi-button-reset',
										'click': function (ev) { handleAction(ev, 'deactivate', olt.mac_addr, onu.id); }
									}, _('Disable'))
								])
							]);
							onuTable.appendChild(row);
						} else {
							// Updates only the necessary cells
							updateText(rowId + '-status', statusText);
							document.getElementById(rowId + '-status').style.color = statusColor;
							updateText(rowId + '-sn', onu.sn === '0000-00000000' ? '-' : onu.sn);
							updateText(rowId + '-uptime', onu.uptime === '0s' ? '-' : onu.uptime);
							updateText(rowId + '-temp', onu.temperature === 0 ? '-' : String(onu.temperature));
							updateText(rowId + '-power', rxTxText);
						}
					}
				}
			} catch (err) {
				console.error(err);
			}
		}, 1);

		return container;
	}
});