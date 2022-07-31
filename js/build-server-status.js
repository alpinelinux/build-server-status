'use strict';

// var MINI = require('minified');
// var _=MINI._, $=MINI.$, $$=MINI.$$, EE=MINI.EE, HTML=MINI.HTML;

const rowTemplate = document.getElementById('template-table-row');
const buildlogsUri = "https://build.alpinelinux.org/buildlogs";
const maxActivityCount = 3;

class BuildServerStatus {
    constructor() {
        const protocol = (window.location.protocol == "http:" ? "ws:" : "wss:")
        this.wsEndpoint = `${protocol}//${window.location.host}/ws/`;
        this.connect(this.wsEndpoint);
        this.subscribers = [];

        const pre = document.createElement('p');
        pre.style.wordWrap = "break-word";

        const mqtt_status = document.getElementById('mqtt_connect_status');
        mqtt_status.appendChild(pre);
        this.statusElem = pre;
    }

    connect(endpoint) {
        const ws = new WebSocket(endpoint);
        ws.addEventListener('open', data => this.open(data));
        ws.addEventListener('close', data => this.close(data));
        ws.addEventListener('message', data => this.msg(data));
        ws.addEventListener('error', data => this.error(data));
    }

    open(e) {
        this.status("CONNECTED", "green");
    }

    error(e) {
        console.error(e);
    }

    msg(e) {
        const data = JSON.parse(e.data);

        for (const subscriber of this.subscribers) {
            subscriber(data);
        }
    }

    close(e) {
        this.status("CONNECTION LOST", "red");
        setTimeout(() => this.connect(this.wsEndpoint), 2000);
    }

    subscribe(callback) {
        this.subscribers.push(callback);
    }

    status(msg, color) {
        this.statusElem.style.color = color;
        this.statusElem.innerText = msg;
    }
}

class BuildServerInterface {
    constructor() {
        this.table = document.getElementById('builds');
        this.builders = {};
        this.builderNr = 1;
    }

    updateStatus(msg) {
        if (msg.Msg == "") {
            return;
        }

        if (this.builders[msg.Builder] == undefined) {
            this.builders[msg.Builder] = new Builder(document.getElementById('servers'), this.builderNr++, msg.Builder);
            this.sortTable();
        }

        const builder = this.builders[msg.Builder];
        builder.update(msg);
    }

    sortTable() {
        const collator = new Intl.Collator([], {numeric: true});

        const serversElem = document.getElementById('servers');
        let servers = [].slice.call(serversElem.children);

        servers.sort(function(a, b) {
            const splitServer = function(name) {
                const parts = name.split("-");
                const len = parts.length;

                return [parts.slice(0, len - 1), parts.slice(len - 1)]
            }
            const serverA = a.getElementsByClassName('host')[0].innerText;
            const serverB = b.getElementsByClassName('host')[0].innerText;

            let [serverAVer, serverAArch] = splitServer(serverA);
            let [serverBVer, serverBArch] = splitServer(serverB);

            let comp = collator.compare(serverAVer, serverBVer);
            if (comp == 0) {
                return collator.compare(serverAArch, serverBArch);
            } else {
                return -1 * comp; // Invert order
            }
        });

        let idx=1;
        for (const server of servers) {
            server.getElementsByClassName('nr')[0].innerText = idx++;
            serversElem.appendChild(server);
        }
    }
}

class Builder {
    constructor(parent, nr, builderName) {
        this.builderName = builderName;
        this.activity = [];

        this.elem = rowTemplate.content.firstElementChild.cloneNode(true);
        this.elem.getElementsByClassName('nr')[0].innerText = nr;
        this.elem.getElementsByClassName('host')[0].innerText = builderName;

        parent.appendChild(this.elem);
    }

    update(msg) {
        switch(msg.MsgType) {
        case "progress":
            let pkgname = msg.PackageName.split("/")[1];
            this.activity.push({
                text: `${msg.PackageName}-${msg.PackageVersion}`,
                url: `${buildlogsUri}/${this.builderName}/${msg.PackageName}/${pkgname}-${msg.PackageVersion}.log`,
            })

            this.updateProgress('prgr_built', msg.BuildProgress);
            this.updateProgress('prgr_total', msg.TotalProgress);
            break;
        case "error":
            this.updateError(msg);
            break;
        case "idle":
            this.activity = [{text: "idle"}];
            this.updateProgress('prgr_built', {Current: 0, Total: 0});
            this.updateProgress('prgr_total', {Current: 0, Total: 0});
            this.updateError({Msg: ""});
            break;
        default:
            this.activity.push({
                text: msg.Msg,
            })
            break;
        }

        this.activity = this.activity.slice(
            Math.max(0, this.activity.length - maxActivityCount),
            maxActivityCount + 1
        );
        this.updateActivity(this.activity);
    }

    updateActivity(activity) {
        let activityContents = activity.map(function(elem) {
            if (elem.url != undefined) {
                return `<a href="${elem.url}">${elem.text}</a>`;
            } else {
                return elem.text;
            }
        })
            .filter(elem => elem != "")
            .join("<br />");

        this.elem.getElementsByClassName('msgs')[0].innerHTML = activityContents;

    }

    updateProgress(cls, progress) {
        this.elem.getElementsByClassName(cls)[0]
            .firstElementChild.setAttribute('value', progress.Current);
        this.elem.getElementsByClassName(cls)[0]
            .firstElementChild.setAttribute('max', progress.Total);

        if (progress.Total == 0) {
            this.elem.getElementsByClassName(cls)[0]
                .getElementsByClassName('progress-value')[0]
                .innerText = "";
        } else {
            let percentage = Math.round(progress.Current / progress.Total * 100);

            this.elem.getElementsByClassName(cls)[0]
                .getElementsByClassName('progress-value')[0]
                .innerText = `${percentage}%`;
        }
    }

    updateError(err) {
        const errElem = this.elem.getElementsByClassName('errmsgs')[0];

        if (err.Msg == "") {
            errElem.innerText = "";
            return
        }

        errElem.innerHTML = `<a href="${err.Logurl}">${err.Reponame}/${err.Pkgname}</a>`
    }
}

let bss = new BuildServerStatus();
let bsi = new BuildServerInterface();
bss.subscribe(msg => bsi.updateStatus(msg));
