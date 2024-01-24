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

$("#btnRun").click(function(){
    $("#loadingSpinner").show();
    // TODO below
    document.getElementById("action").value = "run";
    $.post("", $("#mainForm").serialize(), function(data) {
        console.log(data);
    }, "json")
    .fail(function(data) {
        console.log(data.responseText);
    }).always(function() {
        $("#loadingSpinner").hide();
    });
})

var formCommand = "";

function getCommand() {
    $("#loadingSpinner").show();
    document.getElementById("action").value = "show";
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
