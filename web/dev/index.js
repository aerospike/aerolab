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
            return false;
        }
    });
    return true;
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
        toastr.success("Job started successfully");
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

function showCommandOut(jobId) {
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
                            $("#xlModalTitle").html(ntitle+' <div class="spinner-border" role="status"><span class="sr-only">Loading...</span></div>');
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
  })
{{template "ansiup" .}}
{{end}}
