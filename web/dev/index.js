{{define "mainjs"}}
function pendingActionShowAll(id) {
    let isChecked = $(id).is(":checked");
    console.log(isChecked); // TODO
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

$('.checkForGetParams').each(function() {
    var label = $("label[for='" + $(this).attr('id') + "']");
    if (label.length < 1) {
        return;
    }
    label = label[0].innerText.replace("* ","");
    var labelParam = $.urlParam(label);
    var inputItem = this;
    if (labelParam != null) {
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
{{else}}
function updateCurrentInventoryPage() {}
{{end}}

function updateJobList(setTimer = false, firstRun = false) {
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
            setTimeout(updateJobList, 60000);
        };
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
        action: function ( e, dt, node, config ) {
            dt.ajax.reload(callback = function () {
                toastr.success("Table data refreshed");
            });
        }
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
        buttons: [{extend: 'reload',className: 'btn btn-info',}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/clusters',dataSrc:""},
        columns: [{{$clusters := index .Inventory "Clusters"}}{{range $clusters.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    $('#invclients').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"]],
        fixedColumns: {left: 2, right: 1},
        buttons: [{extend: 'reload',className: 'btn btn-info',}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/clients',dataSrc:""},
        columns: [{{$clients := index .Inventory "Clients"}}{{range $clients.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    $('#invagi').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [];
        },
        order: [],
        fixedColumns: {left: 2, right: 1},
        buttons: [{extend: 'reload',className: 'btn btn-info',}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/agi',dataSrc:""},
        columns: [{{$agi := index .Inventory "AGI"}}{{range $agi.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}],
    });
    $('#invtemplates').DataTable({
        "stateSaveParams": function (settings, data) {
            data.order = [[0,"asc"],[1,"asc"],[2,"asc"],[3,"asc"]];
        },
        order: [[0,"asc"],[1,"asc"],[2,"asc"],[3,"asc"]],
        fixedColumns: {left: 1},
        buttons: [{
            className: 'btn btn-success',
            text: 'Create',
            action: function ( e, dt, node, config ) {
                let url = "{{.WebRoot}}template/create";
                window.location.href = url;
            }},
            {extend: 'reload',className: 'btn btn-info',},
            {
            className: 'btn btn-danger',
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
        buttons: [
            {
                className: 'btn btn-success',
                text: 'Create',
                action: function ( e, dt, node, config ) {
                    let url = "{{.WebRoot}}volume/create";
                    window.location.href = url;
                }
            },{
                className: 'btn btn-warn',
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
            }{{if eq .Backend "gcp"}},{
                className: 'btn btn-info',
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
            }{{end}},{
                extend: 'reload',className: 'btn btn-info',
            }{{if eq .Backend "gcp"}},{
                className: 'btn btn-warning',
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
            }{{end}},{
                className: 'btn btn-danger',
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
        buttons: [{
            className: 'btn btn-success',
            text: 'Create',
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
            }}{{if ne .Backend "docker"}},{
                className: 'btn btn-warning',
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
                }}{{end}},{extend: 'reload',className: 'btn btn-info',},{
                className: 'btn btn-danger',
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
        buttons: [{
            className: 'btn btn-success',
            text: 'Create',
            action: function ( e, dt, node, config ) {
                {{if eq .Backend "aws"}}
                let url = "{{.WebRoot}}config/aws/expiry-install";
                {{end}}
                {{if eq .Backend "gcp"}}
                let url = "{{.WebRoot}}config/gcp/expiry-install";
                {{end}}
                window.location.href = url;
            }},{
                className: 'btn btn-info',
                text: 'Change Frequency',
                action: function ( e, dt, node, config ) {
                    {{if eq .Backend "aws"}}
                    let url = "{{.WebRoot}}config/aws/expiry-run-frequency";
                    {{end}}
                    {{if eq .Backend "gcp"}}
                    let url = "{{.WebRoot}}config/gcp/expiry-run-frequency";
                    {{end}}
                    window.location.href = url;
            }},{extend: 'reload',className: 'btn btn-info',},{
                className: 'btn btn-danger',
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
        buttons: [{extend: 'reload',className: 'btn btn-info',}],
        ajax: {url:'{{.WebRoot}}www/api/inventory/subnets',dataSrc:""},
        columns: [{{$subnets := index .Inventory "Subnets"}}{{range $subnets.Fields}}{ data: '{{.Backend}}{{.Name}}' },{{end}}]
    });
    {{end}}
}
{{else}}
function initDatatable() {
}
{{end}}

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
    updateJobList(true, true);
    initDatatable();
  })
{{template "ansiup" .}}
{{end}}
