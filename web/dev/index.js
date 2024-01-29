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
    commandOutXhr = $.ajax("{{.WebRoot}}job/"+jobId, {
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
    }
    $("#loadingSpinner").show();
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
    $.post("{{.WebRoot}}job/"+jobId, "action="+signal,function(data) {
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

function updateJobList(setTimer = false) {
    $.getJSON("{{.WebRoot}}jobs/", function(data) {
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

$(function () {
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
    getCommand(true);
    updateJobList(true);
  })
{{template "ansiup" .}}
{{end}}
