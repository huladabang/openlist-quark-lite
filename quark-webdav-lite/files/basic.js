'use strict';
'require view';
'require form';
'require uci';

return view.extend({
	load: function() { return uci.load('quark-webdav'); },
	render: function() {
		var m = new form.Map('quark-webdav', _('夸克 WebDAV'),
			_('独立轻量服务，不包含 OpenList。首版仅支持浏览和读取文件。保存后请重启服务。'));
		var s = m.section(form.TypedSection, 'quark_webdav');
		s.anonymous = true;
		s.addremove = false;
		var o = s.option(form.Flag, 'enabled', _('启用'));
		o.rmempty = false;
		o = s.option(form.Value, 'listen', _('监听地址'));
		o.default = '0.0.0.0:5244';
		o.rmempty = false;
		o = s.option(form.Value, 'root_id', _('夸克根目录 ID'));
		o.default = '0';
		o.rmempty = false;
		o.description = _('通常保持 0。');
		o = s.option(form.Value, 'root_path', _('挂载目录路径'));
		o.default = '/';
		o.rmempty = false;
		o.placeholder = '/视频';
		o.description = _('只挂载指定目录，例如 /视频；填写 / 表示整个网盘。名称必须与夸克中的目录完全一致。');
		o = s.option(form.Value, 'parallel', _('最大并发下载数'));
		o.datatype = 'range(1,4)';
		o.default = '3';
		o.rmempty = false;
		o = s.option(form.Value, 'chunk_mb', _('分段大小（MB）'));
		o.datatype = 'range(1,32)';
		o.default = '2';
		o.rmempty = false;
		o.description = _('建议使用 2MB；服务会从 2 路开始，并在客户端等待数据时自动提高到设定的最大并发数。');
		o = s.option(form.Value, 'username', _('WebDAV 用户名'));
		o.default = 'quark';
		o = s.option(form.Value, 'password', _('WebDAV 密码'));
		o.password = true;
		o = s.option(form.Value, 'cookie', _('夸克 Cookie'));
		o.password = true;
		o.rmempty = false;
		o.description = _('从 pan.quark.cn 已登录请求中复制完整 Cookie。凭据只保存在本机配置中。');
		o = s.option(form.DummyValue, '_url', _('WebDAV 地址'));
		o.rawhtml = true;
		o.cfgvalue = function() {
			var port = (uci.get('quark-webdav', 'main', 'listen') || ':5244').split(':').pop();
			return '<code>http://' + window.location.hostname + ':' + port + '/</code>';
		};
		return m.render();
	}
});
