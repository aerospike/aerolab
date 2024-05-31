(function ($) {
    'use strict'
  
    $('.connectedSortable').sortable({
        placeholder: 'sort-highlight',
        connectWith: '.connectedSortable',
        handle: '.card-header, .nav-tabs',
        forcePlaceholderSize: true,
        zIndex: 999999,
        stop: function( event, ui ) { localStore(); }
      })
    $('.connectedSortable .card-header').css('cursor', 'move')
   
    $(function () {
        $('[data-toggle="tooltip"]').tooltip({
            delay: { "show": 200, "hide": 0 }
        })
    })

    $(function () {
        $('[data-toggle="modal"]').tooltip({
            delay: { "show": 200, "hide": 0 }
        })
    })

    function copyToClipboard(contents) {
        if (!navigator.clipboard) {
            const mySmartTextarea = document.createElement('textarea');
            mySmartTextarea.innerHTML = contents;
            document.body.appendChild(mySmartTextarea);
            mySmartTextarea.select();
            document.execCommand('copy');
            document.body.removeChild(mySmartTextarea);
            toastr.success("Copied to clipboard");
        }else {
            navigator.clipboard.writeText(contents).then(
                function() {
                    toastr.success("Copied to clipboard!");
                }
            )
            .catch(
                function() {
                    toastr.error("Copy to clipboard failed!");
                }
            )
        }
    }

    $("#clearAll").on('click', function() {
        if ($(document).find(".execStep").length == 0) {
            toastr.success("List already empty, try adding a script :)")
            return;
        }
        if (!confirm("All unsaved data will be lost. Are you sure?")) {
            return;
        };
        $(".execStep").remove();
        localStore();
        toastr.success("List cleared")
        document.getElementById("emptyShell").style="";
        document.getElementById("fullShell").style="display:none";
    })

    $("#copyToClipboard").on('click', function() {
        var all = getScript();
        copyToClipboard(all.join("\n"));
    })

    $("#saveScript").on('click', function() {
        var all = getScript();
        var script=all.join("\n");
        var downloader = document.getElementById("scriptDownloader");
        downloader.href = 'data:text/plain;charset=utf-8,' + encodeURIComponent(script);
        downloader.click();
    })

    $(document).on('click', '.removeCard', function() {
        var item = $(this).parent().parent().parent();
        removeCard(item);
    })

    function removeCard(item) {
        setTimeout(function() {
            item.remove();
            localStore();
            if ($(document).find(".execStep").length == 0) {
                document.getElementById("emptyShell").style="";
                document.getElementById("fullShell").style="display:none";
            };
        }, 500);
    }

    $("#loadScript").on('click', function() {
        document.getElementById("fromFileLoader").click();
    })

    $("#fromFileLoader").on('change', function() {
        if (this.files.length == 0) {
            return;
        }
        $(".execStep").remove();
        const file = this.files[0];
        var reader = new FileReader();
        reader.onload = function (e) {
            const file = e.target.result;
            var lines = file.split("\n");
            loadScriptContents(lines)
            $("#fromFileLoader").val("");
            localStore();
            toastr.success("Script Loaded");
        };
        reader.onerror = (e) => alert(e.target.error.name);
        reader.readAsText(file);
    })

    function loadScriptContents(lines) {
        var header = "";
        var item = null;
        for (var i=0;i<lines.length;i++) {
            if (lines[i] == "") {
                continue;
            }
            if (lines[i].startsWith("#")) {
                header = lines[i].split(" ## ");
                item = addEntry(header[0].replace(/^# /, ''), header[2], header[1], false);
            }
        }
        genCode();
    }

    function loadOnStart() {
        if (typeof(Storage) !== "undefined") {
            var item = localStorage.getItem("asbenchscript");
            if (item !== null) {
                loadScriptContents(item.split("\n"));
            };
        };
        if ($(document).find(".execStep").length == 0) {
            document.getElementById("emptyShell").style="";
            document.getElementById("fullShell").style="display:none";
        } else {
            document.getElementById("fullShell").style="";
            document.getElementById("emptyShell").style="display:none";
        }        
    }

    loadOnStart();

    $("#enableAerolab").change(function() {
        genCode();
      })

    $("#enableSimple").change(function() {
        genCode();
      })

    $("#enableShell").change(function() {
        genCode();
      })
    
    $('#modal-wait').on('shown.bs.modal', function () {
        $('#waitTitle').focus();
    })  
    $('#modal-wait').on('hidden.bs.modal', function () {
        document.getElementById("waitSave").style='display:none';
        setTimeout(hideTooltip,10);
    })  

    $('#modal-kill').on('shown.bs.modal', function () {
        $('#killTitle').focus();
    })  
    $('#modal-kill').on('hidden.bs.modal', function () {
        document.getElementById("killSave").style='display:none';
        setTimeout(hideTooltip,10);
    })  

    $('#modal-sleep').on('shown.bs.modal', function () {
        $('#sleepTitle').focus();
    })  
    $('#modal-sleep').on('hidden.bs.modal', function () {
        document.getElementById("sleepSave").style='display:none';
        setTimeout(hideTooltip,10);
    })  

    $('#modal-asbench').on('shown.bs.modal', function () {
        colorController(document.getElementById("addAsbechTitle"));
        colorController(document.getElementById("addAsbechWorkload"));
        colorController(document.getElementById("addAsbechWorkloadX"));
        colorController(document.getElementById("addAsbechObject"));
        $('#addAsbechTitle').focus();
    })  
    $('#modal-asbench').on('hidden.bs.modal', function () {
        document.getElementById("saveEditAsbench").style='display:none';
        setTimeout(hideTooltip,10);
    })  

    $('#modal-replaceSeed').on('shown.bs.modal', function () {
        $('#replaceSeedData').focus();
    })  
    $('#modal-replaceAuth').on('shown.bs.modal', function () {
        $('#replaceAuthUser').focus();
    })  
    $('#modal-replaceClientName').on('shown.bs.modal', function () {
        $('#replaceClientNameNew').focus();
    })  

    $('#modal-replaceSeed').on('hidden.bs.modal', function () {
        setTimeout(hideTooltip,10);
    })  
    $('#modal-replaceAuth').on('hidden.bs.modal', function () {
        setTimeout(hideTooltip,10);
    })  
    $('#modal-replaceClientName').on('hidden.bs.modal', function () {
        setTimeout(hideTooltip,10);
    })  

    toastr.options["positionClass"] = "toast-top-center";
})(jQuery)

function stripStep() {
    var all = $(".runStepClean").map(function() {
        $(this).parent().parent().parent().find(".runStep")[0].innerText = this.innerText;
    });
}

function nodeExpander(nodelist) {
    if (nodelist == "all") {
        return nodelist;
    }
    nl = nodelist.split(",");
    var newlist = "";
    for (var j=0;j<nl.length;j++) {
        var item = nl[j];
        if (!item.includes("-")) {
            if (newlist != "" ) {
                newlist = newlist + "," + item;
            } else {
                newlist = item;
            }
        } else {
            var itemRange = item.split("-");
            if ((isNaN(itemRange[0]))||(isNaN(itemRange[1]))||(itemRange.length > 2)) {
                toastr.error("Node list expander error - not a valid list!")
                return nodelist;
            }
            var start = itemRange[0];
            var end = itemRange[1];
            var i = start;
            while (true) {
                if (newlist != "" ) {
                    newlist = newlist + "," + i;
                } else {
                    newlist = i;
                }                    
                i++;
                if (i > end) {
                    break;
                }
            }
        }
    }
    // deduplicate
    newlist = [...new Set(newlist.split(","))].join(",");
    return newlist;
}

function genCode() {
    if ($(document).find(".execStep").length == 0) {
        document.getElementById("emptyShell").style="";
        document.getElementById("fullShell").style="display:none";
    } else {
        document.getElementById("fullShell").style="";
        document.getElementById("emptyShell").style="display:none";
    }
    stripStep();
    if ($("#enableAerolab").is(':checked')) {
        var all = $(".runStep").map(function() {
            if (this.innerText.startsWith("sleep ")) {
                return;
            } else if (this.innerText.startsWith("asbench")) {
                var metaVals = $(this).parent().parent().find(".runStepMeta")[0].innerText.split(";");
                var clientName = "clientName";
                if (metaVals.length > 1) {
                    clientName = metaVals[1];
                }
                var clientList = "all"
                if ((metaVals.length > 2)&&(metaVals[2] != "")) {
                    clientList = metaVals[2];
                }
                this.innerText = `for i in $(seq 1 `+metaVals[0]+`)
do
    aerolab client attach -n `+clientName+` -l `+clientList+` --detach -- /bin/bash -c "run_` + this.innerText.replace("@@CUSTOMSWITCHES@@ ","").replace("@@HOSTNAME@@","\\$(hostname)n\\$(pidof asbench |wc -w)") + `"
done`;
            } else {
                var metaVals = $(this).parent().parent().find(".runStepMeta")[0].innerText.split(";");
                var clientName = "clientName";
                if (metaVals.length > 1) {
                    clientName = metaVals[1];
                }
                var clientList = "all"
                if ((metaVals.length > 2)&&(metaVals[2] != "")) {
                    clientList = metaVals[2];
                }
                this.innerText = `aerolab client attach -n `+clientName+` -l `+clientList+` -- /bin/bash -c '` + this.innerText + `'`;
            }
        });
    } else if ($("#enableShell").is(':checked')) {
        var all = $(".runStep").map(function() {
            if ((this.innerText.startsWith("RET=0"))||(this.innerText.startsWith("sleep "))||(this.innerText.includes("pkill -9 asbench"))) {
                return;
            }
            var countEnd = $(this).parent().parent().find(".runStepMeta")[0].innerText.split(";")[0]-1;
            this.innerText = `start=$(( $(pidof asbench |wc -w) ))
end=$(( \${start} + `+ countEnd +` ))
for i in $(seq \${start} \${end} )
do
    nohup `+this.innerText.replace("@@CUSTOMSWITCHES@@ ","").replace("@@HOSTNAME@@","$(hostname)-${i}")+` >>/var/log/asbench_\${i}.log 2>&1 &
done`;
        });
    } else {
        var all = $(".runStep").map(function() {
            this.innerText = this.innerText.replace("@@CUSTOMSWITCHES@@ ","").replace("@@HOSTNAME@@","$(hostname)");
        });
    }
}

function addEntry(title, code, meta, storeEach=true, uuid="") {
    var master = null;
    if (uuid == "") {
        uuid = crypto.randomUUID();
        master = document.createElement("div");
        master.className = "card execStep";
        master.innerHTML = `
        <div class="card-header">
            <h3 class="card-title">`+title+`<button type="button" class="btn btn-tool" title="Edit" onclick='loadFormFromScript(this.parentNode.parentNode.parentNode);'><i class="fas fa-edit"></i></button></h3>
            <div class="card-tools">
                <button type="button" class="btn btn-tool removeCard" data-card-widget="remove" title="Remove">
                    <i class="fas fa-times"></i>
                </button>
            </div>
        </div>
        <div class="card-body">
            <div hidden style="display:none;"><code><pre class="runStepMeta">`+meta+`</pre></code></div>
            <div hidden style="display:none;"><code><pre class="runStepClean">`+code+`</pre></code></div>
            <div hidden style="display:none;"><code><pre class="runStepUUID">`+uuid+`</pre></code></div>
            <code><pre class="runStep">`+code+`</pre></code>
        </div>`;
        $(".execSteps").append(master);
    } else {
        $(document).find(".runStepUUID").map(function() {
            if (this.innerText != uuid) {
                return;
            }
            $(this).parent().parent().parent().find(".runStepMeta")[0].innerText = meta;
            $(this).parent().parent().parent().find(".runStepClean")[0].innerText = code;
            $($(this).parent().parent().parent().parent().children(".card-header")[0]).children(".card-title")[0].innerHTML = title + `<button type="button" class="btn btn-tool" title="Edit" onclick='loadFormFromScript(this.parentNode.parentNode.parentNode);'><i class="fas fa-edit"></i></button>`;
            master = $(this).parent().parent().parent().parent();
        })
    }
    if (storeEach) {
        genCode();
        localStore();
    };
    return master;
}

function localStore() {
    if (typeof(Storage) !== "undefined") {
        localStorage.setItem("asbenchscript", getScript().join("\n"));
    };
}

function getScript() {
    var all = $(".execStep").map(function() {
        var head = $($(this).children(".card-header")).children(".card-title");
        var headText = head[0].innerText;
        var clean = $($(this).children(".card-body")).find(".runStepClean");
        var cleanText = clean[0].innerText;
        var meta = $($(this).children(".card-body")).find(".runStepMeta");
        var metaText = meta[0].innerText;
        var body = $($(this).children(".card-body")).find(".runStep");
        var bodyText = body[0].innerText;
        return "# "+headText+" ## "+metaText+" ## "+cleanText+"\n"+bodyText+"\n";
    }).get();
    return all;
}

function clearAsbenchForm() {
    // clear form
    document.getElementById("asbenchForm").reset();

    // set the display styles and labels to defaults
    var item=document.getElementById('addAsbechCompRatio');
    item.disabled=true; item.style='background-color: #b1b1b1'; item.value='';
    workloadRelabel(document.getElementById("addAsbechWorkload").parentNode); workloadUpdate();
    item=document.getElementById('addAsbechRackID');
    item.style='background-color: #b1b1b1'; item.disabled=true; item.value='';
    addAsbenchShow();

    // remove all object specs except the first one
    var objects = $(document).find(".asbenchObjectsDef");
    if (objects.length > 1) {
        for (var i=1;i<objects.length;i++) {
            objects[i].remove();
        }
    }

    // clear form
    document.getElementById("asbenchForm").reset();
}

function clearWaitForm() {
    document.getElementById("waitForm").reset();
}

function clearKillForm() {
    document.getElementById("killForm").reset();
}

function clearSleepForm() {
    document.getElementById("sleepForm").reset();
}

function loadAsbenchFormFromScript(title, meta, line, uuid) {
    // extract meta and clear form
    var qty = meta[0];
    var clientName = "";
    if (meta.length > 1) {
        clientName = meta[1];
    }
    var clientList = "";
    if (meta.length > 2) {
        clientList = meta[2];
    }
    clearAsbenchForm();

    // load basics
    document.getElementById("addAsbechQty").value = qty;
    document.getElementById("addAsbechTitle").value = title;
    document.getElementById("addAsbechAerolab").value = clientName;
    document.getElementById("addAsbechClientsList").value = clientList;
    document.getElementById("saveEditAsbench").style='';
    document.getElementById("asbenchEditUUID").value = uuid;

    var objects = "";
    // load line into parameters, also load meta and title
    var all = $(".asbench-switch-type").map(function() {
        var switchType = this.innerText;
        var data = $(this).parent().find(".asbench-switch-data")[0];
        var param = $(this).parent().children(".asbench-switch")[0];
        if ((switchType == "string")||(switchType == "int")) {
            var paramIndex = line.indexOf(" " + param.innerText);
            if (paramIndex == -1) {
                return;
            }
            var paramValueStartIndex = paramIndex + param.innerText.length + 1;
            var paramValueEndIndex = line.indexOf(" ", paramValueStartIndex);
            if ((paramValueEndIndex == -1)||((param.innerText == "@@CUSTOMSWITCHES@@ "))) {
                paramValueEndIndex = line.length;
            }
            data.value = line.substring(paramValueStartIndex,paramValueEndIndex);
            if (param.innerText == "-w ") {
                var dva = data.value.split(",");
                $($(this).parent().children("#asbenchWorkloadsDef")[0]).find(".workloadsSelect")[0].value = dva[0];
                workloadRelabel(document.getElementById("addAsbechWorkload").parentNode); workloadUpdate();
                if (dva.length > 1) {
                    document.getElementById("addAsbechWorkloadX").value = dva[1];
                };
                workloadUpdate();
            } else if (param.innerText == "-o ") {
                objects = data.value.substring(1, data.value.length-1).split(",");
            }
        } else {
            if (line.indexOf(" " + data.innerText) == -1) {
                $(data).parent().find("select")[0].value = "No";
            } else {
                $(data).parent().find("select")[0].value = "Yes";
            }
        }
    });

    // load object types - special
    var odef = $(document).find(".asbenchObjectsDef")[0];
    var start = true;
    for (var i=0;i<objects.length;i++) {
        var obj = odef;
        if (!start) {
            obj=document.getElementById('asbenchObjectsDef').cloneNode(true); document.getElementById('asbenchObjects').appendChild(obj);
        }
        start = false;
        var item = objects[i];
        var count = "";
        if (item.includes("*")) {
            count = item.split("*")[0];
            item = item.split("*")[1];
        }
        var nType = item.charAt(0);
        var nVal = "";
        if (item.length > 1) {
            nVal = item.substring(1);
        }
        if ((nType.includes("["))||(nType.includes("]"))||(nVal.includes("["))||(nVal.includes("]"))) {
            nType = "";
            nVal = "CDT - unsupported by this tool"
        }
        // TODO: modify obj with values from objects[i]
        $(obj).find(".objectCount")[0].value = count;
        $(obj).find(".objectsSelect")[0].value = nType;
        $(obj).find(".objectSpread")[0].value = nVal;
        if (nVal != "") {
            $($(obj).find(".objectSpread")[0]).parent().css("display","");
        } else {
            $($(obj).find(".objectSpread")[0]).parent().css("display","none");
        }
    }

    // finish loading and show form
    var item=document.getElementById('addAsbechCompRatio'); if (document.getElementById('addAsbechCompress').value == 'Yes') { item.disabled=false; item.style=''; } else { item.disabled=true; item.style='background-color: #b1b1b1'; item.value=''; }
    var item=document.getElementById('addAsbechRackID'); if (document.getElementById('addAsbechReadReplica').value == 'preferRack') { item.style=''; item.disabled=false; } else { item.style='background-color: #b1b1b1'; item.disabled=true; item.value=''; }
    addAsbenchShow();
    $('#modal-asbench').modal('show');
}

function loadWaitFormFromScript(title, meta, line, uuid) {
    clearWaitForm();
    var clientName = "";
    if (meta.length > 1) {
        clientName = meta[1];
    }
    var clientList = "";
    if (meta.length > 2) {
        clientList = meta[2];
    }
    document.getElementById("waitTitle").value = title;
    document.getElementById("waitClientName").value = clientName;
    document.getElementById("waitClients").value = clientList;
    document.getElementById("waitSave").style='';
    document.getElementById("waitEditUUID").value = uuid;
    $('#modal-wait').modal('show');
}

function loadKillFormFromScript(title, meta, line, uuid) {
    clearKillForm();
    var clientName = "";
    if (meta.length > 1) {
        clientName = meta[1];
    }
    var clientList = "";
    if (meta.length > 2) {
        clientList = meta[2];
    }
    document.getElementById("killTitle").value = title;
    document.getElementById("killClientName").value = clientName;
    document.getElementById("killClients").value = clientList;
    document.getElementById("killSave").style='';
    document.getElementById("killEditUUID").value = uuid;
    $('#modal-kill').modal('show');
}

function loadSleepFormFromScript(title, meta, line, uuid) {
    clearSleepForm();
    document.getElementById("sleepTitle").value = title;
    document.getElementById("sleepTime").value = meta[0];
    document.getElementById("sleepEditUUID").value = uuid;
    document.getElementById("sleepSave").style='';
    $('#modal-sleep').modal('show');
}

function loadFormFromScript(node) {
    var head = $($(node).children(".card-header")).children(".card-title");
    var headText = head[0].innerText;
    var clean = $($(node).children(".card-body")).find(".runStepClean");
    var cleanText = clean[0].innerText;
    var meta = $($(node).children(".card-body")).find(".runStepMeta");
    var metaText = meta[0].innerText;
    var uuido = $($(node).children(".card-body")).find(".runStepUUID");
    var uuid = uuido[0].innerText;

    if ((metaText.startsWith("0;"))&&(cleanText.startsWith("RET=0"))) {
        loadWaitFormFromScript(headText, metaText.split(";"), cleanText, uuid);
    } else if (cleanText.startsWith("sleep ")) {
        loadSleepFormFromScript(headText, metaText.split(";"), cleanText, uuid);
    } else if (cleanText.includes("kill -9 asbench")) {
        loadKillFormFromScript(headText, metaText.split(";"), cleanText, uuid);
    } else {
        loadAsbenchFormFromScript(headText, metaText.split(";"), cleanText, uuid);
    }
}

function hideTooltip() {
    $('[data-toggle="tooltip"]').tooltip('hide');
    $('[data-toggle="modal"]').tooltip('hide');
    $(".nav-link").map(function() {
        $(this).blur();
    });
}

function colorController(that) {
    if (that.disabled) {
        return;
    }
    if (that.value == "") {
        that.style="background-color: #FFB6C1";
    } else {
        that.style="";
    }
}