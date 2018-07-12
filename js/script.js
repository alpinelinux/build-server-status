var MINI = require('minified');
var _=MINI._, $=MINI.$, $$=MINI.$$, EE=MINI.EE, HTML=MINI.HTML;

var wsHost = "msg.alpinelinux.org";
var wsPort = "8083";
var wsSub = "build/#";
var max_mqtt_msgs_count = 3;
var buildlogsUri = "http://build.alpinelinux.org/buildlogs";

function connect(){
	// Create a client instance
	var clientId = Math.random().toString(36).substr(2, 22); // length - 20 symb
	var client = new Paho.MQTT.Client(wsHost, Number(wsPort), clientId);

	// set callback handlers
	client.onConnectionLost = onConnectionLost;
	client.onMessageArrived = onMessageArrived;

	// connect the client
	client.connect({onSuccess:onConnect,onFailure:onFailure});

	// called when the client connects
	function onConnect() {
		// Once a connection has been made, make a subscription
		client.subscribe(wsSub);
		echo('CONNECTED', false, 'green');
	}

	function onFailure() {
		echo('Sorry, the websocket at "' + wsHost + '" is unavailable.', false, 'red');
		setTimeout('connect();', 2000);
	}

	// called when the client looses its connection
	function onConnectionLost(responseObject) {
		if (responseObject.errorCode !== 0) {
			echo('CONNECTION LOST<br>' + responseObject.errorMessage, false, 'red');
			setTimeout('connect();', 2000);
		}
	}

	// called when a message arrives
	function onMessageArrived(message) {
		var host = message.destinationName || null;
		if (host == null)
			return;
		var a = host.split("/");
		var err = false;
		if (a[2] == "errors") {
			err = true;
		}
		host = a[1];
		if (host && host.match(/^build-/))
			mqtt_msg(host,message.payloadString,err);
	}

	setTimeout('sort_table();', 1500);
}

function echo(msg,mqtt_msg,color){
	var pre = document.createElement("p");
	pre.style.wordWrap = "break-word";
	if (color) pre.style.color = color;
	pre.innerHTML = msg;

	var output = (mqtt_msg ? document.getElementById("mqtt_msgs") : document.getElementById("mqtt_connect_status"));
	if (output) output.appendChild(pre);

	var max_msg_count = ( mqtt_msg ? max_mqtt_msgs_count : 1 );
	if (output && output.childNodes.length > max_msg_count) {
		output.removeChild(output.firstChild);
	}
}

function mqtt_msg(host, msg, err){
	id = "bs_"+host;
	if (!$('#servers #'+id).length) {
		$('#servers').add(EE('tr', {'@id':id}, [
			EE('td', {'className': 'nr'}, $('#servers tr').length+1),
			EE('td', {'className': 'host'}, host),
			EE('td', {'className': 'msgs_container'}),
			EE('td', {'className': 'errmsgs_container'}),

			EE('td', {'className': 'prgr_built'}),
			EE('td', {'className': 'prgr_total'}),
		]));
		$('#'+id+' .msgs_container').add(EE('div', {'className': 'msgs'}));
		$('#'+id+' .errmsgs_container').add(EE('div', {'className': 'errmsgs'}));
	}

	if (msg == "idle") {
		$('#'+id+' .msgs').ht(''); // clear previous messages
	}else if ($('#'+id+' .msgs span').length >= max_mqtt_msgs_count) {
		$('#'+id+' .msgs span')[0].remove();
		$('#'+id+' .msgs br')[0].remove();
	}

	var pat = /^(\d+)\/(\d+)\s+(\d+)\/(\d+)\s+(.*)/i
	if (err) {
		if (msg == null) {
			$('#'+id+' .errmsgs').ht(''); // clear previous messages
			return;
		}
		var obj = JSON.parse(msg);
		var errmsg = EE('a', {'href': obj.logurl, 'className':"errmsgs"}, obj.reponame+"/"+obj.pkgname);
		if ($('#'+id+' .errmsgs span').length >= max_mqtt_msgs_count) {
			$('#'+id+' .errmsgs span')[0].remove();
			$('#'+id+' .errmsgs br')[0].remove();
		}
		$('#'+id+' .errmsgs').add([EE('span', errmsg), EE('br')]);
		return;
	} else if (msg.match(pat)) {
		var msg_arr = msg.match(pat);
		var built_curr = msg_arr[1];
		var built_last = msg_arr[2];
		var repo_curr = msg_arr[3];
		var repo_last = msg_arr[4];
		var built_pr = Math.round(built_curr*100/built_last);
		var repo_pr = Math.round(repo_curr*100/repo_last);

		msg = EE('a', {'href': buildlog_url(host, msg_arr[5])}, msg_arr[5]);

		$('#'+id+' .prgr_built').ht('<progress value="'+built_curr+'" max="'+built_last+'"></progress> <span class="progress-value">'+built_pr+'%</span>');
		$('#'+id+' .prgr_total').ht('<progress value="'+repo_curr+'" max="'+repo_last+'"></progress> <span class="progress-value">'+repo_pr+'%</span>');
	} else {
		$('#'+id+' .prgr_built').ht('<progress value="0" max="100"></progress> <span class="progress-value">&nbsp; &nbsp; &nbsp; &nbsp; &nbsp;</span>');
		$('#'+id+' .prgr_total').ht('<progress value="0" max="100"></progress> <span class="progress-value">&nbsp; &nbsp; &nbsp; &nbsp; &nbsp;</span>');
	}

	$('#'+id+' .msgs').add([EE('span', msg), EE('br')]);
}

function buildlog_url(host, msg) {
	var tokens = msg.match(/^([^/]+)\/(\S+) (.*)/);
	var repo = tokens[1];
	var pkgname = tokens[2];
	var pkgver = tokens[3];

	return [buildlogsUri, host, repo, pkgname, pkgname].join('/') + '-' + pkgver + '.log';
}

function sort_table(){
	var host_header = document.getElementById( 'host' ).getElementsByTagName( 'a' )[0];
	if (host_header) host_header.click(); // this will sort table by host column

	// update order nr's after sorting
	$('#servers tr').each(function(obj, index) {
		$('#'+obj.id+' .nr').ht(index+1);
	});
}

window.addEventListener("load", connect, false);