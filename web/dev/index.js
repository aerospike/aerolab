{{define "mainjs"}}
function menuSetActive(id) {
    // get current item
    let current = $(id);
    // if current item is an expanding menu item, we are not setting anything; exit
    if (current.parent().attr("class").split(/\s+/).includes("menu")) {
        return;
    }
    // fade in loading spinner
    $("#loadingSpinner").fadeIn();
    // reset: remove all active classes
    let menu = $("#mainMenu")
    menu.find("a").each(function () {
        $(this).removeClass("active");
        $(this).removeClass("bg-blue");
        $(this).removeClass("bg-white");
    });
    // add active class to currently selected item
    current.addClass("active");
    current.addClass("bg-blue");
    // recursively add active class to parent menu items
    let menuId = menu.attr("id");
    while (current.attr("id") != menuId) {
        current = current.parent();
        if ($(current).attr("class").split(/\s+/).includes("menu")) {
            $(current).children("a").each(function () {
                $(this).addClass("active");
                $(this).addClass("bg-white");
            });
        }
    }
}

function pendingActionShowAll(id) {
    let isChecked = $(id).is(":checked");
    console.log(isChecked); // TODO
}

var formCommand = "";

$("#btnRun").click(function(){
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
        $(document).Toasts('create', {
            class: 'bg-danger',
            title: 'ERROR',
            subtitle: data.statusText,
            body: body
        })
    })
    .always(function() {
        $("#loadingSpinner").hide();
    });
})

var commandOutXhr = false;

function hideCommandOut() {
    if (commandOutXhr != false) {
        commandOutXhr.abort();
    }
}

function showCommandOut(jobId) {
    $("#modal-xl").modal("show");
    $("#xlModalTitle").html('Loading <div class="spinner-border" role="status"><span class="sr-only">Loading...</span></div>');
    $("#xlModalBody").text("");
    var last_response_len = false;
    var isLog = false;
    var ntitle = "";
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
                    $("#xlModalBody").append(this_response);
                } else {
                    var lines = this_response.split("\n");
                    for(var i = 0;i < lines.length;i++){
                        if (isLog) {
                            $("#xlModalBody").append(lines[i]+"\n");
                        } else if (lines[i].includes("-=-=-=-=- [Log] -=-=-=-=-")) {
                            isLog = true;
                            if (lines[i+1] == "") {
                                i++;
                            }
                        } else if (lines[i].includes("-=-=-=-=- [cmdline]")) {
                            let ncmdline = lines[i].replace(/^-=-=-=-=- \[cmdline\]/,"").replace(/ -=-=-=-=-$/,"");
                            $("#xlModalBody").append("$" + ncmdline+"\n\n");
                        } else if (lines[i].includes("-=-=-=-=- [command]")) {
                            ntitle = lines[i].replace(/^-=-=-=-=- \[command\]/,"").replace(/ -=-=-=-=-$/,"");
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
    });
}

function getCommand() {
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
        $(document).Toasts('create', {
            class: 'bg-danger',
            title: 'ERROR',
            subtitle: data.statusText,
            body: body
        })
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

$(function () {
    $('[data-toggle="tooltip"]').tooltip({ trigger: "hover", placement: "right", boundary: "viewport" });
    $('[data-toggle="tooltipleft"]').tooltip({ trigger: "hover", placement: "left", boundary: "viewport" });
    $('[data-toggle="tooltiptop"]').tooltip({ trigger: "hover", placement: "top", boundary: "viewport" });
    $('.select2bs4').select2({
        theme: 'bootstrap4'
    })
    $(".select2bs4tag").select2({
        theme: 'bootstrap4',
        tags: true,
        tokenSeparators: [',', ' ']
    })
    getCommand();
  })
{{end}}
