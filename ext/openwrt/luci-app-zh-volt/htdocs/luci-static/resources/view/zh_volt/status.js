'use strict';
'require view';
'require request';
'require poll';
'require ui';

return view.extend({
	render: function () {
		return E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('Status da OLT/ONU')),
			E('div', { 'class': 'cbi-map-descr' }, _('Monitorização em tempo real via zh-volt.')),
			E('table', { 'class': 'table', 'id': 'onu_status' }, [
				E('tr', { 'class': 'tr' }, [
					E('th', { 'class': 'th' }, _('ID')),
					E('th', { 'class': 'th' }, _('Status')),
					E('th', { 'class': 'th' }, _('SN')),
					E('th', { 'class': 'th' }, _('Uptime'))
				])
			])
		]);
	},

	update: function (node, data) {
		var table = node.querySelector('#onu_status');
		// Limpa linhas antigas e popula com dados da sua API Rust (porta 8081)
		request.get('http://' + window.location.hostname + ':8081/').then(function (res) {
			var olts = res.json();
			// Lógica para iterar sobre olts[].onus e atualizar a tabela
		});
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});