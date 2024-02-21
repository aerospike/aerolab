{{define "mainjs"}}
function pendingActionShowAll(id) {
    let isChecked = $(id).is(":checked");
    console.log(isChecked); // not implemented in this version
}

$('.aerolab-required').on("change",function() {
    handleRequiredFieldColor(this);
});
$('.aerolab-required').on("keyup",function() {
    handleRequiredFieldColor(this);
});
function handleRequiredFieldColor(item) {
    if ($(item).val() == "") {
        $(item).addClass("is-invalid");
    } else {
        $(item).removeClass("is-invalid");
    }
}

$('.aerolab-required').each(function() {
    handleRequiredFieldColor(this);
});

$.urlParam = function(name){
    var results = new RegExp('[\?&]' + name + '=([^&#]*)').exec(window.location.href);
    if (results==null) {
       return null;
    }
    return decodeURI(results[1]) || 0;
}

$('.checkForGetParamsSelect').each(function() {
    var valname = $(this).attr("valname");
    var paramVal = $.urlParam(valname);
    if (paramVal == null) {
        return;
    }
    var item = this;
    $(item).val([paramVal]);
});

$('.checkForGetParams').each(function() {
    var label = $("label[for='" + $(this).attr('id') + "']");
    if (label.length < 1) {
        return;
    }
    label = label[0].innerText.replace("* ","");
    var labelParam = $.urlParam(label);
    var inputItem = this;
    var inputOptional = $('#isSet-' + $(this).attr('id'));
    if (labelParam != null) {
        $(inputItem).removeAttr("hidden");
        if (inputOptional.length > 0) {
            inputOptional.val('yes');
        }
        if (labelParam == "discover-caller-ip") {
            $.getJSON("https://api.ipify.org?format=json",
            function (data) {
                $(inputItem).val(data.ip);
                handleRequiredFieldColor(inputItem);
            })
            .fail(function() {
                $(inputItem).val(labelParam);
            });
        } else {
            $(this).val(labelParam);
            handleRequiredFieldColor(this);
        };
    };
});

$('.checkForGetParamsToggle').each(function() {
    var label = $("label[for='" + $(this).attr('id') + "']");
    if (label.length < 1) {
        return;
    }
    label = label[0].innerText.replace("* ","");
    var labelParam = $.urlParam(label);
    var inputItem = this;
    if (labelParam != null) {
        if (labelParam == "on") {
            $(this).attr('checked','checked');
        } else if (labelParam == "off") {
            $(this).removeAttr('checked');
        };
        $(this).val(labelParam);
    };
});

function checkRequiredFields() {
    var fieldsOk = true;
    $('.aerolab-required').each(function() {
        if ($(this).val() == "") {
            toastr.error($(this).attr("name").replace(/^xx/,"").replaceAll("xx",".").replace(/^\./,"")+" is required")
            $(this).trigger("focus");
            $('html').animate(
                {
                  scrollTop: $(this).offset().top-200,
                },
                500 //speed
            );
            fieldsOk = false;
            return false;
        }
    });
    return fieldsOk;
}

var formCommand = "";

$("#btnRun1").click(function(){
    btnRun();
})

$("#btnRun2").click(function(){
    btnRun();
})

function btnRun() {
    if (!checkRequiredFields()) {
        return;
    }
    $("#loadingSpinner").show();
    document.getElementById("action").value = "run";
    document.getElementById("useShortSwitches").value = document.getElementById("shortSwitches").checked;
    $.post("", $("#mainForm").serialize(), function(data) {
        showCommandOut(data);
    })
    .fail(function(data) {
        let body = data.responseText;
        if ((data.status == 0)&&(body == undefined)) {
            body = "Connection Error";
        }
        toastr.error(data.statusText+": "+body);
    })
    .always(function() {
        $("#loadingSpinner").hide();
    });
}

var commandOutXhr = false;

function hideCommandOut() {
    if (commandOutXhr != false) {
        commandOutXhr.abort();
    }
}

$("#modal-xl").on("hidden.bs.modal", function () {
    hideCommandOut();
});

function showCommandOut(jobId, runningJob=true) {
    if (runningJob) {
        updateJobList();
    }
    $("#xlModalSpinner").hide();
    $("#modal-xl").modal("show");
    $("#xlModalTitle").html('Loading <div class="spinner-border" role="status"><span class="sr-only">Loading...</span></div>');
    $("#xlModalBody").text("");
    $("#abrtJobId").val(jobId);
    $("#abrtButton").show();
    var last_response_len = false;
    var isLog = false;
    var ntitle = "";
    var ansi_up = new AnsiUp();
    commandOutXhr = $.ajax("{{.WebRoot}}www/api/job/"+jobId, {
        xhrFields: {
            onprogress: function(e)
            {
                var this_response, response = e.currentTarget.response;
                if(last_response_len === false)
                {
                    this_response = response;
                    last_response_len = response.length;
                }
                else
                {
                    this_response = response.substring(last_response_len);
                    last_response_len = response.length;
                }
                if (isLog) {
                    $("#xlModalBody").append(ansi_up.ansi_to_html(this_response));
                } else {
                    var lines = this_response.split("\n");
                    for(var i = 0;i < lines.length;i++){
                        if (isLog) {
                            $("#xlModalBody").append(ansi_up.ansi_to_html(lines[i])+"\n");
                        } else if (lines[i].includes("-=-=-=-=- [Log] -=-=-=-=-")) {
                            isLog = true;
                            if (lines[i+1] == "") {
                                i++;
                            }
                        } else if (lines[i].includes("-=-=-=-=- [cmdline]")) {
                            let ncmdline = lines[i].replace(/^-=-=-=-=- \[cmdline\]/,"").replace(/ -=-=-=-=-$/,"");
                            $("#xlModalBody").append("$" + ncmdline+"\n\n");
                        } else if (lines[i].includes("-=-=-=-=- [command]")) {
                            ntitle = "aerolab " + lines[i].replace(/^-=-=-=-=- \[command\]/,"").replace(/ -=-=-=-=-$/,"");
                            $("#xlModalTitle").html('<div class="spinner-border" role="status"><span class="sr-only">Loading...</span></div> '+ntitle);
                        };
                    }
                };
            }
        }
    }, "text")
    .fail(function(data) {
        if (data.statusText == "abort") {
            return;
        }
        let body = data.responseText;
        if ((data.status == 0)&&(body == undefined)) {
            body = "Connection Error";
        }
        toastr.error("ERROR: "+body);
    })
    .always(function() {
        commandOutXhr = false;
        $("#xlModalTitle").html(ntitle);
        $("#abrtButton").hide();
        if (runningJob) {
            updateJobList();
        };
    });
}

function getCommand(supressError=false) {
    if (!supressError) {
        if (!checkRequiredFields()) {
            return;
        }
        $("#loadingSpinner").show();
    }
    document.getElementById("action").value = "show";
    document.getElementById("useShortSwitches").value = document.getElementById("shortSwitches").checked;
    $.post("", $("#mainForm").serialize(), function(data) {
        var switches = false;
        var inner = "";
        for (let i=0; i<data.length;i++) {
            if (data[i].startsWith("-")) {
                switches = true;
                inner = inner+"<span class=\"na\">"+data[i]+"</span> ";
            } else if (!switches) {
                inner = inner+"<span class=\"nv\">"+data[i]+"</span> ";
            } else {
                inner = inner+"<span class=\"s\">"+data[i]+"</span> ";
            }
        }
        document.getElementById("cmdBuilder").innerHTML = inner;
        formCommand = data.join(" ");
    }, "json")
    .fail(function(data) {
        let body = data.responseText;
        if ((data.status == 0)&&(body == undefined)) {
            body = "Connection Error";
        }
        toastr.error(data.statusText+": "+body);
    })
    .always(function(data) {
        $("#loadingSpinner").hide();
    });
}

$("#btnShowCommand").click(function(){ 
    getCommand();
})

$("#btnCopyCommand").click(function(){ 
    navigator.clipboard.writeText(formCommand);
    toastr.success("Copied to clipboard");
})

$("#btnCopyLog").click(function(){ 
    navigator.clipboard.writeText($("#xlModalBody").text());
    toastr.success("Copied to clipboard");
})

$("#abrtSigInt").click(function(){ 
    abortCommand("sigint");
})

$("#abrtSigKill").click(function(){ 
    abortCommand("sigkill");
})

function abortCommand(signal) {
    $("#xlModalSpinner").show();
    jobId = $("#abrtJobId").val();
    $.post("{{.WebRoot}}www/api/job/"+jobId, "action="+signal,function(data) {
        console.log(data);
    }, "text")
    .fail(function(data) {
        let body = data.responseText;
        if ((data.status == 0)&&(body == undefined)) {
            body = "Connection Error";
        }
        toastr.error(data.statusText+": "+body);
    })
    .always(function(data) {
        $("#xlModalSpinner").hide();
    });
}

function getTooltipPlacement() {
    let formSize = $("#formSize")
    let pctSize = $(formSize).css("max-width").replace("%","");
    if (pctSize <= 85) {
        return "left";
    }
    return "top";
}

$( window ).on( "resize", function() {
    $('[data-toggle="tooltipleft"]').tooltip('dispose');
    $('[data-toggle="tooltipleft"]').tooltip({ trigger: "hover", placement: getTooltipPlacement(), fallbackPlacement:["bottom"], boundary: "viewport" });
} );

{{if .IsInventory}}
function updateCurrentInventoryPage() {
    $("#custom-tabs-one-tabContent").find(".tab-pane").each(function(index, item) {
        if (!$(item).hasClass("active")) {
            return;
        }
        let tables = $(item).find('table');
        for (let i = 0; i < tables.length; i++) {
            if ($(tables[i]).attr("id") == undefined) {
                continue;
            }
            let dt = $(tables[i]).DataTable();
            if (dt.ajax == undefined) {
                continue;
            }
            dt.ajax.reload();
        }
    })
}
$(window).on('focus', function () {
    updateJobList(false, false);
});
{{else}}
function updateCurrentInventoryPage() {}
{{end}}

class Mutex {
    constructor() {
      this._locking = Promise.resolve();
      this._locked = false;
    }
  
    isLocked() {
      return this._locked;
    }
  
    lock() {
      this._locked = true;
      let unlockNext;
      let willLock = new Promise(resolve => unlockNext = resolve);
      willLock.then(() => this._locked = false);
      let willUnlock = this._locking.then(() => unlockNext);
      this._locking = this._locking.then(() => willLock);
      return willUnlock;
    }
}

var jobListMutex = new Mutex();
function updateJobList(setTimer = false, firstRun = false) {
    mutexunlock = jobListMutex.lock();
    $.getJSON("{{.WebRoot}}www/api/jobs/", function(data) {
        document.getElementById("pending-action-count").innerText = data["RunningCount"];
        if (data["HasRunning"]) {
            $("#pending-action-icon").removeClass("fa-bell");
            $("#pending-action-icon").removeClass("far");
            $("#pending-action-icon").removeClass("fa-lg");
            $("#pending-action-icon").addClass("fa-solid");
            $("#pending-action-icon").addClass("fa-circle-notch");
            $("#pending-action-icon").addClass("fa-spin");
            $("#pending-action-count").removeClass("badge-danger");
            $("#pending-action-count").removeClass("badge-success");
            if (!$("#pending-action-count").hasClass("badge-warning")) {
                $("#pending-action-count").addClass("badge-warning");
            }
        } else {
            $("#pending-action-icon").removeClass("fa-solid");
            $("#pending-action-icon").removeClass("fa-circle-notch");
            $("#pending-action-icon").removeClass("fa-spin");
            $("#pending-action-icon").addClass("fa-bell");
            $("#pending-action-icon").addClass("far");
            $("#pending-action-icon").addClass("fa-lg");
            if (data["HasFailed"]) {
                $("#pending-action-count").removeClass("badge-warning");
                $("#pending-action-count").removeClass("badge-success");
                if (!$("#pending-action-count").hasClass("badge-danger")) {
                    $("#pending-action-count").addClass("badge-danger");
                }
            } else {
                $("#pending-action-count").removeClass("badge-warning");
                $("#pending-action-count").removeClass("badge-danger");
                if (!$("#pending-action-count").hasClass("badge-success")) {
                    $("#pending-action-count").addClass("badge-success");
                }
            }
        }
        $(".jobslist").each(function(index,item){
            $(item).empty();
        });
        var ln1 = '<div class="dropdown-divider"></div><a href="#" onclick="showCommandOut(';
        // jobId in single quotes, comma, isRunning
        var ln2 = ');" class="dropdown-item"><i class=';
        // lnicon
        var ln3 = '></i> ';
        // command
        var ln4 = '<span class="float-right text-muted text-sm">';
        // startedWhen
        //var ln3 = '</span><br><span class="text-muted text-sm">&nbsp;</span><span class="text-muted float-right text-sm">&nbsp;</span></a>';
        var ln5 = '</span></a>';
        if (data.Jobs != null) {
            for (var i=0; i<data.Jobs.length;i++) {
                let jl = $(".jobslist");
                let lnicon = '"'+data.Jobs[i]["Icon"]+' mr-2"';
                if (lnicon == "") {
                    lnicon = '"fas fa-check mr-2" style="color: #01b27d;"';
                    if (data.Jobs[i]["IsFailed"]) {
                        lnicon = '"fas fa-xmark mr-2" style="color: #ee2b2b;"';
                    }
                    if (data.Jobs[i]["IsRunning"]) {
                        lnicon = '"fas fa-spinner fa-spin mr-2"';
                    }
                } else {
                    if (data.Jobs[i]["IsFailed"]) {
                        lnicon = lnicon + ' style="color: #ee2b2b;"';
                    } else if (data.Jobs[i]["IsRunning"]) {
                        //lnicon = '"'+data.Jobs[i]["Icon"]+' fa-spin mr-2"';
                        lnicon = '"fas fa-spinner fa-spin mr-2"';
                    } else {
                        lnicon = lnicon + ' style="color: #01b27d;"';
                    }
                }
                jobid="'"+data.Jobs[i]["RequestID"]+"',"+data.Jobs[i]["IsRunning"];
                $(jl).append(ln1+jobid+ln2+lnicon+ln3+data.Jobs[i]["Command"]+ln4+data.Jobs[i]["StartedWhen"]+ln5);
            }
        };
        if (!firstRun) {
            updateCurrentInventoryPage();
        };
    })
    .fail(function(data) {
        let body = data.responseText;
        if ((data.status == 0)&&(body == undefined)) {
            body = "Connection Error";
        }
        toastr.error(data.statusText+": "+body);
    })
    .always(function(data) {
        if (setTimer) {
            setTimeout(updateJobList, 10000);
        };
        mutexunlock();
    });
}

function clearNotifications() {
    var ts = new Date().toISOString();
    Cookies.set('aerolab_history_truncate', ts, { expires: 360, path: '{{.WebRoot}}' });
    updateJobList();
}

{{if .IsInventory}}
$('a[data-toggle="pill"]').on('shown.bs.tab', function (e) {
    if (tabInit) {
        tabInit = false;
        return;
    }
    Cookies.set('aerolab_inventory_tab', $(e.target).attr("id"), { expires: 360, path: '{{.WebRoot}}' })
    var tab = $(e.target).attr("href") // activated tab
    let tables = $(tab).find('table');
    for (let i = 0; i < tables.length; i++) {
        if ($(tables[i]).attr("id") == undefined) {
            continue;
        }
        let dt = $(tables[i]).DataTable();
        if (dt.ajax == undefined) {
            continue;
        }
        dt.ajax.reload();
    }
});

function initDatatable() {
    $.fn.dataTable.ext.errMode = 'alert';
    $.fn.dataTable.ext.buttons.reload = {
        text: 'Refresh',
        titleAttr: 'Refresh table list from server',
        className: 'dtTooltip',
        action: function ( e, dt, node, config ) {
            dt.ajax.reload(callback = function () {
                toastr.success("Table data refreshed");
            });
        }
    };
    $.fn.dataTable.ext.buttons.myspacer = {
        extend: 'spacer',
        text: '&nbsp;',
        style: 'empty', // empty|bar
    };
    Object.assign(DataTable.defaults, {
        paging: false,
        scrollCollapse: true,
        scrollY: '70vh',
        scrollX: true,
        stateSave: true,
        fixedHeader: true,
        select: true,
        dom: 'Bfrtip',
    });
    $('#invclusters').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"]],
        fixedColumns: {left: 2, right: 1},
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},{
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Go to form: Create New Cluster',
            text: 'Create',
            action: function ( e, dt, node, config ) {
                let url = "{{.WebRoot}}cluster/create";
                window.location.href = url;
            }},
            {extend: 'myspacer'},
            {
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Go to form: Grow Cluster',
            text: 'Grow',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length != 1) {
                    toastr.error("Select one row.");
                    return;
                }
                let data = arr[0];
                let url = "{{.WebRoot}}cluster/grow?ClusterName="+data["ClusterName"];
                window.location.href = url;
            }},
            {extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-warn dtTooltip',
                titleAttr: 'Start / stop instance(s) and aerospike',
                text: 'Nodes',
                buttons: [
                    {
                        className: 'dtTooltip',
                        text: 'Start',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"start","type":"cluster"};
                            if (confirm("Start "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                    {
                        text: 'Stop',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"stop","type":"cluster"};
                            if (confirm("Stop "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                ]
            },
            {extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-warn dtTooltip',
                titleAttr: 'Perform aerospike service actions on node(s)',
                text: 'Aerospike',
                buttons: [
                    {
                        text: 'Start',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"aerospikeStart","type":"cluster"};
                            if (confirm("Start aerospike on "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                    {
                        text: 'Stop',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"aerospikeStop","type":"cluster"};
                            if (confirm("Stop aerospike on "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                    {
                        text: 'Restart',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"aerospikeRestart","type":"cluster"};
                            if (confirm("Restart aerospike on "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                    {
                        text: 'Status',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"aerospikeStatus","type":"cluster"};
                            if (confirm("Status of aerospike on "+arr.length+" nodes")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                ]
            },
            {extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-warn dtTooltip',
                titleAttr: 'Open forms for common configuration actions',
                text: 'Configure',
                buttons: [
                    {
                        text: 'Rack ID',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length < 1) {toastr.error("Select one or more rows.");return;}
                            var cname = arr[0]["ClusterName"];
                            var nodes = [];
                            for (let i=0;i<arr.length;i++) {
                                if (arr[i]["ClusterName"] != cname) {toastr.error("All selected nodes must belong to the same cluster for this action.");return;};
                                nodes.push(arr[i]["NodeNo"]);
                            }
                            window.location.href = "{{.WebRoot}}conf/rackid?ClusterName="+arr[0]["ClusterName"]+"&Nodes="+nodes.join(',');
                        }
                    },
                    {
                        text: 'Namespace Memory',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length < 1) {toastr.error("Select one or more rows.");return;}
                            var cname = arr[0]["ClusterName"];
                            var nodes = [];
                            for (let i=0;i<arr.length;i++) {
                                if (arr[i]["ClusterName"] != cname) {toastr.error("All selected nodes must belong to the same cluster for this action.");return;};
                                nodes.push(arr[i]["NodeNo"]);
                            }
                            window.location.href = "{{.WebRoot}}conf/namespace-memory?ClusterName="+arr[0]["ClusterName"]+"&Nodes="+nodes.join(',');
                        }
                    },
                    {
                        text: 'Fix HB Mesh',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length < 1) {toastr.error("Select one or more rows.");return;}
                            var cname = arr[0]["ClusterName"];
                            var nodes = [];
                            for (let i=0;i<arr.length;i++) {
                                if (arr[i]["ClusterName"] != cname) {toastr.error("All selected nodes must belong to the same cluster for this action.");return;};
                                nodes.push(arr[i]["NodeNo"]);
                            }
                            window.location.href = "{{.WebRoot}}conf/fix-mesh?ClusterName="+arr[0]["ClusterName"]+"&Nodes="+nodes.join(',');
                        }
                    },
                ]
            },
            {{if ne .Backend "docker"}}
            {extend: 'myspacer'},
            {
                className: 'btn btn-warning dtTooltip',
                titleAttr: 'Open form: Extend node(s) expiry time',
                text: 'Extend Expiry',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                    if (arr.length < 1) {toastr.error("Select one or more rows.");return;}
                    let ans = prompt("New Expiry","30h0m0s");
                    if (ans == null) {
                        return;
                    }
                    let data = {"list": arr,"action":"extendExpiry","type":"cluster","expiry":ans};
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                    .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                    .always(function() {$("#loadingSpinner").hide();});
                }
            },    
            {{end}}
            {extend: 'myspacer'},
            {extend: 'reload',className: 'btn btn-info',},
            {extend: 'myspacer'},
            {
            className: 'btn btn-danger dtTooltip',
            titleAttr: 'Destroy node instance(s)',
            text: 'Destroy',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length == 0) {
                    toastr.error("Select one or more rows first");
                    return;
                }
                let data = {"list": arr,"action":"destroy","type":"cluster"};
                if (confirm("Remove "+arr.length+" nodes")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {
                        showCommandOut(data);
                    })
                    .fail(function(data) {
                        let body = data.responseText;
                        if ((data.status == 0)&&(body == undefined)) {
                            body = "Connection Error";
                        }
                        toastr.error(data.statusText+": "+body);
                    })
                    .always(function() {
                        $("#loadingSpinner").hide();
                    });
                }
            }}
        ],
        ajax: {url:'{{.WebRoot}}www/api/inventory/clusters',dataSrc:""},
        columns: [{{$clusters := index .Inventory "Clusters"}}{{range $clusters.Fields}}{ data: '{{.Backend}}{{.Name}}'{{if eq .Name "Firewalls"}}, render: function (data, type, row, meta) {
            return data.join("<br>");
        }{{end}}{{if eq .Name "InstanceRunningCost"}}, render: function (data, type, row, meta) {
            return "$" + Math.round(data*10000)/10000;
        }{{end}}{{if eq .Name "IsRunning"}}, render: function (data, type, row, meta) {
            let disabledString = 'success"';
            if (!data) {
                disabledString = 'default" disabled';
            }
            return '<button type="button" class="btn btn-block btn-'+disabledString+' onclick="xRunAttach('+"this,'cluster','"+row["ClusterName"]+"','"+row["NodeNo"]+"'"+","+meta.row+');">Attach</button>';
        }{{end}} },{{end}}]
    });
    $('#invclients').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"]],
        fixedColumns: {left: 2, right: 1},
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-success dtTooltip',
                titleAttr: 'Go to form: create new client machine of given type',
                text: 'Create',
                buttons: [
                    {
                        text: 'Vanilla',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/none";
                        }
                    },
                    {
                        text: 'Base',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/base";
                        }
                    },
                    {
                        text: 'AerospikeTools',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/tools";
                        }
                    },
                    {
                        text: 'AMS',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/ams";
                        }
                    },
                    {
                        text: 'VSCode',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/vscode";
                        }
                    },
                    {
                        text: 'Trino',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/trino";
                        }
                    },
                    {
                        text: 'ElasticSearch',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/elasticsearch";
                        }
                    },
                    {
                        text: 'RestGateway',
                        action: function ( e, dt, node, config ) {
                            window.location.href = "{{.WebRoot}}client/create/rest-gateway";
                        }
                    },
                ]
            },
            {extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-success dtTooltip',
                titleAttr: 'Got to form: grow client machine set, adding a given machine type',
                text: 'Grow',
                buttons: [
                    {
                        text: 'Vanilla',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/none?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'Base',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/base?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'AerospikeTools',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/tools?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'AMS',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/ams?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'VSCode',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/vscode?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'Trino',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/trino?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'ElasticSearch',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/elasticsearch?ClientName="+arr[0]["ClientName"];
                        }
                    },
                    {
                        text: 'RestGateway',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length != 1) {toastr.error("Select one row.");return;}
                            window.location.href = "{{.WebRoot}}client/create/rest-gateway?ClientName="+arr[0]["ClientName"];
                        }
                    },
                ]
            },
            {extend: 'myspacer'},
            {
                extend: 'collection',
                className: 'custom-html-collection btn-warn dtTooltip',
                titleAttr: 'Start/Stop given instance(s)',
                text: 'Nodes',
                buttons: [
                    {
                        text: 'Start',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"start","type":"client"};
                            if (confirm("Start "+arr.length+" clients")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                    {
                        text: 'Stop',
                        action: function ( e, dt, node, config ) {
                            let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                            if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                            let data = {"list": arr,"action":"stop","type":"client"};
                            if (confirm("Stop "+arr.length+" clients")) {
                                $("#loadingSpinner").show();
                                $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                                .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                                .always(function() {$("#loadingSpinner").hide();});
                            }
                        }
                    },
                ]
            },
            {{if ne .Backend "docker"}}
            {extend: 'myspacer'},
            {
                className: 'btn btn-warning dtTooltip',
                titleAttr: 'Go to form: extend expiry of instance(s)',
                text: 'Extend Expiry',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                    if (arr.length < 1) {toastr.error("Select one or more rows.");return;}
                    let ans = prompt("New Expiry","30h0m0s");
                    if (ans == null) {
                        return;
                    }
                    let data = {"list": arr,"action":"extendExpiry","type":"client","expiry":ans};
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                    .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                    .always(function() {$("#loadingSpinner").hide();});
                }
            },    
            {{end}}
            {extend: 'myspacer'},
            {extend: 'reload',className: 'btn btn-info',},
            {extend: 'myspacer'},
            {
            className: 'btn btn-danger dtTooltip',
            titleAttr: 'Destroy instance(s)',
            text: 'Destroy',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length == 0) {
                    toastr.error("Select one or more rows first");
                    return;
                }
                let data = {"list": arr,"action":"destroy","type":"client"};
                if (confirm("Remove "+arr.length+" machines")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {
                        showCommandOut(data);
                    })
                    .fail(function(data) {
                        let body = data.responseText;
                        if ((data.status == 0)&&(body == undefined)) {
                            body = "Connection Error";
                        }
                        toastr.error(data.statusText+": "+body);
                    })
                    .always(function() {
                        $("#loadingSpinner").hide();
                    });
                }
            }}
        ],
        ajax: {url:'{{.WebRoot}}www/api/inventory/clients',dataSrc:""},
        columns: [{{$clients := index .Inventory "Clients"}}{{range $clients.Fields}}{ data: '{{.Backend}}{{.Name}}'{{if eq .Name "Firewalls"}}, render: function (data, type, row, meta) {
            return data.join("<br>");
        }{{end}}{{if eq .Name "AccessUrl"}}, render: function (data, type, row, meta) {
            return '<a href="'+data+'" target="_blank">'+data+'</a>';
        }{{end}}{{if eq .Name "InstanceRunningCost"}}, render: function (data, type, row, meta) {
            return "$" + Math.round(data*10000)/10000;
        }{{end}}{{if eq .Name "IsRunning"}}, render: function (data, type, row, meta) {
            let disabledString = 'success"';
            if (!data) {
                disabledString = 'default" disabled';
            }
            return '<button type="button" class="btn btn-block btn-'+disabledString+' onclick="xRunAttach('+"this,'client','"+row["ClientName"]+"','"+row["NodeNo"]+"'"+","+meta.row+');">Attach</button>';
        }{{end}} },{{end}}]
    });
    $('#invagi').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [];
        },
        order: [],
        fixedColumns: {left: 2, right: 1},
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},
            {
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Got to form: Create a new AGI instance',
            text: 'Create',
            action: function ( e, dt, node, config ) {
                let url = "{{.WebRoot}}agi/create";
                window.location.href = url;
            }},
            {extend: 'myspacer'},
            {
            className: 'btn btn-warn dtTooltip',
            titleAttr: 'Start a stopped AGI instance',
            text: 'Start',
            action: function ( e, dt, node, config ) {
                let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                let data = {"list": arr,"action":"start","type":"agi"};
                if (confirm("Start "+arr.length+" agi")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                    .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                    .always(function() {$("#loadingSpinner").hide();});
                }
            }},
            {extend: 'myspacer'},
            {
            className: 'btn btn-warn dtTooltip',
            titleAttr: 'Stop a running AGI instance',
            text: 'Stop',
            action: function ( e, dt, node, config ) {
                let arr = [];dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {arr.push(this.data());});
                if (arr.length == 0) {toastr.error("Select one or more rows first");return;}
                let data = {"list": arr,"action":"stop","type":"agi"};
                if (confirm("Stop "+arr.length+" agi")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {showCommandOut(data);})
                    .fail(function(data) {let body = data.responseText;if ((data.status == 0)&&(body == undefined)) {body = "Connection Error";};toastr.error(data.statusText+": "+body);})
                    .always(function() {$("#loadingSpinner").hide();});
                }
            }},            
            {extend: 'myspacer'},
            {extend: 'reload',className: 'btn btn-info',},
            {extend: 'myspacer'},
            {
            className: 'btn btn-danger dtTooltip',
            titleAttr: 'Remove an existing AGI instance',
            text: 'Destroy',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length == 0) {
                    toastr.error("Select one or more rows first");
                    return;
                }
                let data = {"list": arr,"action":"destroy","type":"agi"};
                if (confirm("Remove "+arr.length+" agi")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {
                        showCommandOut(data);
                    })
                    .fail(function(data) {
                        let body = data.responseText;
                        if ((data.status == 0)&&(body == undefined)) {
                            body = "Connection Error";
                        }
                        toastr.error(data.statusText+": "+body);
                    })
                    .always(function() {
                        $("#loadingSpinner").hide();
                    });
                }
            }},
            {extend: 'myspacer'},
            {
            className: 'btn btn-danger dtTooltip',
            titleAttr: 'Remove an existing AGI instance and delete an existing AGI persistent data volume',
            text: 'Delete',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length == 0) {
                    toastr.error("Select one or more rows first");
                    return;
                }
                let data = {"list": arr,"action":"delete","type":"agi"};
                if (confirm("Remove "+arr.length+" agi and delete their persistent volumes")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/nodes", JSON.stringify(data), function(data) {
                        showCommandOut(data);
                    })
                    .fail(function(data) {
                        let body = data.responseText;
                        if ((data.status == 0)&&(body == undefined)) {
                            body = "Connection Error";
                        }
                        toastr.error(data.statusText+": "+body);
                    })
                    .always(function() {
                        $("#loadingSpinner").hide();
                    });
                }
            }},
            {
                extend: 'spacer',
                text: '',
                style: 'bar', // empty|bar
            },
            {
                extend: 'collection',
                className: 'custom-html-collection btn-info dtTooltip',
                titleAttr: 'AGI instance actions',
                text: 'Node',
                buttons: [
                    {
                        text: 'Status',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                                let data = this.data();
                                arr.push(data);
                            });
                            if (arr.length != 1) {
                                toastr.error("Select one row.");
                                return;
                            }
                            let data = arr[0];
                            let url = "{{.WebRoot}}agi/status?ClusterName="+data["Name"];
                            window.location.href = url;            
                        }
                    },
                    {
                        text: 'Details',
                        action: function ( e, dt, node, config ) {
                            // TODO background run and report in a nice way: including old-new-name mapping, errors, etc
                            alert('not implemented yet');
                        }
                    },
                    {
                        text: 'Get share link',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                                let data = this.data();
                                arr.push(data);
                            });
                            if (arr.length != 1) {
                                toastr.error("Select one row.");
                                return;
                            }
                            let data = arr[0];
                            let url = "{{.WebRoot}}agi/add-auth-token?ClusterName="+data["Name"]+"&GenURL=on";
                            window.location.href = url;  
                        }
                    },
                    {
                        text: 'Change label',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                                let data = this.data();
                                arr.push(data);
                            });
                            if (arr.length != 1) {
                                toastr.error("Select one row.");
                                return;
                            }
                            let data = arr[0];
                            let url = "{{.WebRoot}}agi/change-label?ClusterName="+data["Name"]+"&Gcpzone="+data["Zone"];
                            window.location.href = url;            
                        }
                    },
                    {
                        text: 'Rerun ingest',
                        action: function ( e, dt, node, config ) {
                            let arr = [];
                            dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                                let data = this.data();
                                arr.push(data);
                            });
                            if (arr.length != 1) {
                                toastr.error("Select one row.");
                                return;
                            }
                            let data = arr[0];
                            let url = "{{.WebRoot}}agi/run-ingest?ClusterName="+data["Name"];
                            window.location.href = url;            
                        }
                    },
                ]
            },
        ],
        ajax: {url:'{{.WebRoot}}www/api/inventory/agi',dataSrc:""},
        columns: [{{$agi := index .Inventory "AGI"}}{{range $agi.Fields}}{ data: '{{.Backend}}{{.Name}}'{{if eq .Name "Firewalls"}}, render: function (data, type, row, meta) {
            return data.join("<br>");
        }{{end}}{{if eq .Name "Status"}}, render: function (data, type, row, meta) {
            if (data == 'READY, HasErrors') {
                return '<span style="color: #fac400;"><i class="fa-solid fa-check"></i><i class="fa-solid fa-triangle-exclamation"></i>&nbsp;</span>';
            }
            if (data == 'READY') {
                return '<i class="fa-solid fa-check" style="color: #00c000;"></i>&nbsp;';
            }
            if (data.startsWith('ERR:')) {
                return '<span style="color: #ff0000;"><i class="fa-solid fa-xmark"></i>&nbsp;'+data+'</span>';
            }
            if (data == '') {
                return '&nbsp;';
            }
            return data;
        }{{end}}{{if eq .Name "RunningCost"}}, render: function (data, type, row, meta) {
            return "$" + Math.round(data*10000)/10000;
        }{{end}}{{if eq .Name "IsRunning"}}, render: function (data, type, row, meta) {
            let disabledString = 'success"';
            if (!data) {
                disabledString = 'default" disabled';
            }
            return '<button type="button" class="btn btn-block btn-'+disabledString+' onclick="xRunAttach('+"this,'agi','"+row["Name"]+"','1'"+","+meta.row+",'"+row["AccessURL"]+"'"+');">Connect</button>';
        }{{end}} },{{end}}],
    });
    $('#invtemplates').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"],[2,"asc"],[3,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"],[2,"asc"],[3,"asc"]],
        fixedColumns: {left: 1},
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},{
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Create a template without running CreateCluster',
            text: 'Create Cluster',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length != 1) {
                    toastr.error("Select one row.");
                    return;
                }
                let data = arr[0];
                let url = "{{.WebRoot}}cluster/create?AerospikeVersion="+data["AerospikeVersion"]+"&DistroName="+data["Distribution"]+"&DistroVersion="+data["OSVersion"];
                window.location.href = url;
            }},
            {extend: 'myspacer'},
            {
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Create a template without running CreateCluster',
            text: 'Create Template',
            action: function ( e, dt, node, config ) {
                let url = "{{.WebRoot}}template/create";
                window.location.href = url;
            }},
            {extend: 'myspacer'},
            {extend: 'reload',className: 'btn btn-info',},
            {extend: 'myspacer'},
            {
            className: 'btn btn-danger dtTooltip',
            titleAttr: 'Delete template(s)',
            text: 'Delete',
            action: function ( e, dt, node, config ) {
                let arr = [];
                dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                    let data = this.data();
                    arr.push(data);
                });
                if (arr.length == 0) {
                    toastr.error("Select one or more rows first");
                    return;
                }
                let data = {"list": arr}
                if (confirm("Remove "+arr.length+" templates")) {
                    $("#loadingSpinner").show();
                    $.post("{{.WebRoot}}www/api/inventory/templates", JSON.stringify(data), function(data) {
                        showCommandOut(data);
                    })
                    .fail(function(data) {
                        let body = data.responseText;
                        if ((data.status == 0)&&(body == undefined)) {
                            body = "Connection Error";
                        }
                        toastr.error(data.statusText+": "+body);
                    })
                    .always(function() {
                        $("#loadingSpinner").hide();
                    });
                }
            }}],
        ajax: { url:'{{.WebRoot}}www/api/inventory/templates', dataSrc:"" },
        columns: [{{$templates := index .Inventory "Templates"}}{{range $templates.Fields}}{ data: '{{.Name}}' },{{end}}]
    });
    {{ if ne .Backend "docker" }}
    $('#invvolumes').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [0,"asc"];
        },
        order: [0,"asc"],
        fixedColumns: {left: 1},
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},
            {
                className: 'btn btn-success dtTooltip',
                titleAttr: 'Got to form: new volume',
                text: 'Create',
                action: function ( e, dt, node, config ) {
                    let url = "{{.WebRoot}}volume/create";
                    window.location.href = url;
                }
            },
            {extend: 'myspacer'},
            {
                className: 'btn btn-warn dtTooltip',
                titleAttr: 'Go to form: mount volume to instance',
                text: 'Mount',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length != 1) {
                        toastr.error("Select one row.");
                        return;
                    }
                    let data = arr[0];
                    let url = "{{.WebRoot}}volume/mount?Name="+data["Name"];
                    window.location.href = url;
                }
            }{{if eq .Backend "gcp"}},{extend: 'myspacer'},{
                className: 'btn btn-info dtTooltip',
                titleAttr: 'Grow given volume size',
                text: 'Grow',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length != 1) {
                        toastr.error("Select one row.");
                        return;
                    }
                    let data = arr[0];
                    let url = "{{.WebRoot}}volume/grow?Name="+data["Name"]+"&Zone="+data["AvailabilityZoneName"];
                    window.location.href = url;
                }
            }{{end}},{extend: 'myspacer'},{
                extend: 'reload',className: 'btn btn-info',
            }{{if eq .Backend "gcp"}},{extend: 'myspacer'},{
                className: 'btn btn-warning dtTooltip',
                titleAttr: 'Go to form: detach volume from instance',
                text: 'Detach',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length != 1) {
                        toastr.error("Select one row.");
                        return;
                    }
                    let data = arr[0];
                    let url = "{{.WebRoot}}volume/detach?Name="+data["Name"]+"&Zone="+data["AvailabilityZoneName"];
                    window.location.href = url;
                }
            }{{end}},{extend: 'myspacer'},{
                className: 'btn btn-danger dtTooltip',
                titleAttr: 'Delete a volume',
                text: 'Delete',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length == 0) {
                        toastr.error("Select one or more rows first");
                        return;
                    }
                    let data = {"list": arr}
                    if (confirm("Remove "+arr.length+" volumes")) {
                        $("#loadingSpinner").show();
                        $.post("{{.WebRoot}}www/api/inventory/volumes", JSON.stringify(data), function(data) {
                            showCommandOut(data);
                        })
                        .fail(function(data) {
                            let body = data.responseText;
                            if ((data.status == 0)&&(body == undefined)) {
                                body = "Connection Error";
                            }
                            toastr.error(data.statusText+": "+body);
                        })
                        .always(function() {
                            $("#loadingSpinner").hide();
                        });
                    }
                }
            }
        ],
        ajax: {url:'{{.WebRoot}}www/api/inventory/volumes',dataSrc:""},
        columns: [{{$vols := index .Inventory "Volumes"}}{{range $vols.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    {{end}}
    $('#invfirewalls').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"]],
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},{
            className: 'btn btn-success dtTooltip',
            text: 'Create',
            {{if eq .Backend "aws"}}
            titleAttr: 'Go to form: New Security Group',
            {{end}}
            {{if eq .Backend "gcp"}}
            titleAttr: 'Go to form: New Firewall Rule',
            {{end}}
            {{if eq .Backend "docker"}}
            titleAttr: 'Go to form: New Network',
            {{end}}
            action: function ( e, dt, node, config ) {
                {{if eq .Backend "aws"}}
                let url = "{{.WebRoot}}config/aws/create-security-groups";
                {{end}}
                {{if eq .Backend "gcp"}}
                let url = "{{.WebRoot}}config/gcp/create-firewall-rules";
                {{end}}
                {{if eq .Backend "docker"}}
                let url = "{{.WebRoot}}config/docker/create-network";
                {{end}}
                window.location.href = url;
            }}{{if ne .Backend "docker"}},{extend: 'myspacer'},{
                className: 'btn btn-warning dtTooltip',
                titleAttr: 'Lock incoming IP of a firewall',
                text: 'Lock IP',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length != 1) {
                        toastr.error("Select one row.");
                        return;
                    }
                    let data = arr[0];
                    {{if eq .Backend "aws"}}
                    let url = "{{.WebRoot}}config/aws/lock-security-groups?NamePrefix="+data["AWS"]["SecurityGroupName"]+"&VPC="+data["AWS"]["VPC"]+"&IP=discover-caller-ip";
                    {{end}}
                    {{if eq .Backend "gcp"}}
                    let url = "{{.WebRoot}}config/gcp/lock-firewall-rules?NamePrefix="+data["GCP"]["FirewallName"]+"&IP=discover-caller-ip";
                    {{end}}
                    window.location.href = url;
                }}{{end}},{extend: 'myspacer'},{extend: 'reload',className: 'btn btn-info',},{extend: 'myspacer'},{
                className: 'btn btn-danger dtTooltip',
                titleAttr: 'Delete selected item(s)',
                text: 'Delete',
                action: function ( e, dt, node, config ) {
                    let arr = [];
                    dt.rows({selected: true}).every(function(rowIdx, tableLoop, rowLoop) {
                        let data = this.data();
                        arr.push(data);
                    });
                    if (arr.length == 0) {
                        toastr.error("Select one or more rows first");
                        return;
                    }
                    let data = {"list": arr}
                    if (confirm("Remove "+arr.length+" items")) {
                        $("#loadingSpinner").show();
                        $.post("{{.WebRoot}}www/api/inventory/firewalls", JSON.stringify(data), function(data) {
                            showCommandOut(data);
                        })
                        .fail(function(data) {
                            let body = data.responseText;
                            if ((data.status == 0)&&(body == undefined)) {
                                body = "Connection Error";
                            }
                            toastr.error(data.statusText+": "+body);
                        })
                        .always(function() {
                            $("#loadingSpinner").hide();
                        });
                    }
                }}
            ],
        ajax: {url:'{{.WebRoot}}www/api/inventory/firewalls',dataSrc:""},
        columns: [{{$fw := index .Inventory "FirewallRules"}}{{range $fw.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    {{ if ne .Backend "docker" }}
    $('#invexpiry').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [0,"asc"];
        },
        order: [0,"asc"],
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},{
            className: 'btn btn-success dtTooltip',
            titleAttr: 'Create and install an automated instance expiry system',
            text: 'Create',
            action: function ( e, dt, node, config ) {
                {{if eq .Backend "aws"}}
                let url = "{{.WebRoot}}config/aws/expiry-install";
                {{end}}
                {{if eq .Backend "gcp"}}
                let url = "{{.WebRoot}}config/gcp/expiry-install";
                {{end}}
                window.location.href = url;
            }},{extend: 'myspacer'},{
                className: 'btn btn-info dtTooltip',
                titleAttr: 'Change run frequency of expiry checker',
                text: 'Change Frequency',
                action: function ( e, dt, node, config ) {
                    {{if eq .Backend "aws"}}
                    let url = "{{.WebRoot}}config/aws/expiry-run-frequency";
                    {{end}}
                    {{if eq .Backend "gcp"}}
                    let url = "{{.WebRoot}}config/gcp/expiry-run-frequency";
                    {{end}}
                    window.location.href = url;
            }},{extend: 'myspacer'},{extend: 'reload',className: 'btn btn-info',},{extend: 'myspacer'},{
                className: 'btn btn-danger dtTooltip',
                titleAttr: 'Remove automated instance expiry system',
                text: 'Remove',
                action: function ( e, dt, node, config ) {
                    {{if eq .Backend "aws"}}
                    let url = "{{.WebRoot}}config/aws/expiry-remove";
                    {{end}}
                    {{if eq .Backend "gcp"}}
                    let url = "{{.WebRoot}}config/gcp/expiry-remove";
                    {{end}}
                    window.location.href = url;
            }}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/expiry',dataSrc:""},
        columns: [{{$expirysystem := index .Inventory "ExpirySystem"}}{{range $expirysystem.Fields}}{ data: '{{.Name}}' },{{end}}]
    });
    {{end}}
    {{if eq .Backend "aws"}}
    $('#invsubnets').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[3,"asc"],[6,"asc"]];
        },
        order: [[3,"asc"],[6,"asc"]],
        buttons: [{extend: "colvis", text:"Columns",className:"btn btn-default dtTooltip",titleAttr:"Choose which columns to show"},{extend: 'myspacer'},{extend: 'reload',className: 'btn btn-info',}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/subnets',dataSrc:""},
        columns: [{{$subnets := index .Inventory "Subnets"}}{{range $subnets.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    {{end}}
}
{{else}}
function initDatatable() {
}
{{end}}

var cliTimer = null;

function timedUpdateCommand() {
    if (cliTimer != null) {
        clearTimeout(cliTimer);
    };
    cliTimer = setTimeout(refreshCliCommand, 1500);
}

function refreshCliCommand() {
    getCommand(true);
}

$('.updateCommandCli').on('change', function() {
    timedUpdateCommand();
})

$('.updateCommandCli').on('keyup', function() {
    timedUpdateCommand();
})

function xRunAttach(tbutton, target, name, node, row, accessURL="") {
    // workaround - prevent selection on button click
    let table = "";
    switch (target) {
        case "cluster":
            table = '#invclusters';
            break;
        case "client":
            table = '#invclients';
            break;
        case "agi":
            table = '#invagi';
            break;
    }
    let t = $(table).DataTable();
    if (t.row(row).selected()) { t.row(row).deselect() } else { t.row(row).select() };
    console.log("target:"+target+" name:"+name+" node:"+node+" row:"+row+" accessURL:"+accessURL);

    if (target == "agi") {
        $(tbutton).addClass("disabled");
        var tbText = $(tbutton).text();
        $(tbutton).html('<span class="fa-solid fa-circle-notch fa-spin"></span>');
        $.post("{{.WebRoot}}www/api/inventory/agi/connect", "name="+name, function(data) {
            var url = accessURL+"?AGI_TOKEN="+data
            window.open(url, '_blank').focus();
        })
        .fail(function(data) {
            let body = data.responseText;
            if ((data.status == 0)&&(body == undefined)) {
                body = "Connection Error";
            }
            toastr.error(data.statusText+": "+body);
        })
        .always(function() {
            $(tbutton).removeClass("disabled");
            $(tbutton).text(tbText);
        });
        return;
    }
    window.open('{{.WebRoot}}www/api/inventory/'+target+'/connect?name='+name+"&node="+node, '_blank').focus();
}

var tabInit = true;
$(function () {
    {{if .IsInventory}}
    var selTab = Cookies.get('aerolab_inventory_tab');
    if (selTab != null && selTab != undefined && selTab != "" && !$("#"+selTab).hasClass("active")) {
        $("#"+selTab).tab("show");
    } else {
        $($("#inventoryTabs").find(".inventoryTab")[0]).tab("show");
    }
    {{end}}
    $('[data-toggle="tooltip"]').tooltip({ trigger: "hover", placement: "right", fallbackPlacement:["bottom","top"], boundary: "viewport" });
    $('[data-toggle="tooltipleft"]').tooltip({ trigger: "hover", placement: getTooltipPlacement(), fallbackPlacement:["bottom"], boundary: "viewport" });
    $('[data-toggle="tooltiptop"]').tooltip({ trigger: "hover", placement: "top", fallbackPlacement:["left","right"], boundary: "viewport" });
    $('.select2bs4').select2({
        theme: 'bootstrap4'
    })
    $(".select2bs4tag").select2({
        theme: 'bootstrap4',
        tags: true,
        tokenSeparators: [',', ' ']
    })
    {{if .IsForm}}getCommand(true);{{end}}
    initDatatable();
    updateJobList(true, true);
    $('.dtTooltip').tooltip({ trigger: "hover", placement: "bottom", fallbackPlacement:["right","top"], boundary: "viewport" });
  })

// filebrowser - server-side
    function getHomeDir(path = '') {
        var homedir = {};
        $.ajax({
            async: false,
            type: 'GET',
            data: {
                'path': path,
            },
            url: '{{.WebRoot}}www/api/homedir',
            success: function(data) {
                homedir = data;
            }
        });
        return homedir;
    }
    function getPathItems(path) {
        var items = {};
        $.ajax({
            async: false,
            type: 'GET',
            dataType: "json",
            data: {
                'path': path,
            },
            url: '{{.WebRoot}}www/api/ls',
            success: function(data) {
                items = data;
            },
            error: function(data) {
                if (data.responseText == "GOUP") {
                    setTimeout(browser[1].up, 100);
                    return items;
                }
            }
        });
        return items;
    }
    var browser = null;
    $(".filebrowser-destroy").click(destroyBrowser);
    function getBrowser(inputbox, inputTitle) {
        if (browser != null) {
            destroyBrowser();
        }
        $("#filebrowser-wrapper").html('<div id="filebrowser" title="'+inputTitle+'"></div>');
        browser = [$('#filebrowser').dialog({
            width: 600,
            height: 480
        })];
        $(".ui-dialog-titlebar-close").addClass('filebrowser-destroy').removeClass("ui-dialog-titlebar-close").html('<span class="fa-solid fa-xmark"></span>');
        browser.push(browser[0].browse({
            root: '/',
            separator: '/',
            contextmenu: true,
            menu: function(type) {
                if (type == 'li') {
                    return {
                        'alert': function($li) {
                            alert('alert for item "' + $li.text() + '"');
                        }
                    }
                }
            },
            dir: function(path) {
                return new Promise(function(resolve, reject) {
                    dir = getPathItems(path);
                    if ($.isPlainObject(dir)) {
                        var result = {files:[], dirs: []};
                        Object.keys(dir).forEach(function(key) {
                            if (typeof dir[key] == 'string') {
                                result.files.push(key);
                            } else if ($.isPlainObject(dir[key])) {
                                result.dirs.push(key);
                            }
                        });
                        resolve(result);
                    } else {
                        reject();
                    }
                });
            },
            exists: function(path) {
                return typeof getPathItems(path) != 'undefined';
            },
            error: function(message) {
                alert(message);
            },
            open: function(filename) {
                browserFileSelected(filename, inputbox);
            },
            on_change: function() {
                $('#path').val(this.path());
            }
        }));
        setTimeout(function() {
            let nval = ''
            if ($(inputbox).val() != "") {
                nval = $(inputbox).val();
            } else if ($(inputbox).attr('placeholder') != "") {
                nval = $(inputbox).attr('placeholder');
            }
            browser[1].show(getHomeDir(nval));
        },10);
    };
    function destroyBrowser() {
        if (browser != null) {
            browser[1].destroy();
            browser[0].dialog('destroy').remove();
            browser = null;
        }
    }
    function browserFileSelected(f, inputbox) {
        $(inputbox).val(f);
        destroyBrowser();
    }

{{template "ansiup" .}}
{{end}}
