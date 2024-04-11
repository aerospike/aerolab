(function ($) {
$("#addAsbench").on('click', function() {
    addAsbench();
})
$("#saveEditAsbench").on('click', function() {
    addAsbench(true);
})

$("#replaceAuthDo").on('click', function() {
    replaceAuthDo();
})

$("#replaceSeedDo").on('click', function() {
    replaceSeedDo();
})

$("#replaceClientNameDo").on('click', function() {
    replaceClientNameDo();
})

$("#menuAddAsbench").on('click', function() {
    clearAsbenchForm();
    addAsbenchShow();
})

$("#waitAdd").on('click', function() {
    addWait();
})
$("#waitSave").on('click', function() {
    addWait(true);
})

$("#killAdd").on('click', function() {
    addKill();
})
$("#killSave").on('click', function() {
    addKill(true);
})

$("#sleepAdd").on('click', function() {
    addSleep();
})
$("#sleepSave").on('click', function() {
    addSleep(true);
})
})(jQuery)

function replaceClientNameDo() {
    var newName = document.getElementById("replaceClientNameNew");
    var newValue = newName.value;
    if (newValue == "") {
        toastr.error("Specify a valid client name for aerolab!");
        return;
    }
    newName.value = "";
    var all = $(".runStepMeta").map(function() {
        var vals = this.innerText.split(";")
        if (vals.length == 1) {
            vals.push(newValue);
        } else {
            vals[1] = newValue;
        }
        this.innerText = vals.join(";");
    });
    genCode();
    localStore();
    toastr.success("Client name modified!");
    $('#modal-replaceClientName').modal('hide');
}

function replaceAuthDo() {
    var user=document.getElementById("replaceAuthUser");
    var userValue = user.value;
    var pass=document.getElementById("replaceAuthPass");
    var passValue = pass.value;
    var replaceString = ""
    if (userValue != "") {
        replaceString = " -U '" + userValue + "' '-P" + passValue + "'"
    }
    user.value="";
    pass.value="";
    var all = $(".runStep").map(function() {
        this.innerText = this.innerText.replace(/ -U '[^ ]+ '-P[^ ]+/,"");
        if (replaceString != "") {
            this.innerText = this.innerText.replace(/ -h [^ ]+/,function(x) {return x+replaceString});
        };
    });
    var all = $(".runStepClean").map(function() {
        this.innerText = this.innerText.replace(/ -U '[^ ]+ '-P[^ ]+/,"");
        if (replaceString != "") {
            this.innerText = this.innerText.replace(/ -h [^ ]+/,function(x) {return x+replaceString});
        };
    });
    localStore();
    toastr.success("Auth values modified!");
    $('#modal-replaceAuth').modal('hide');
}

function replaceSeedDo() {
    var seed=document.getElementById("replaceSeedData");
    var seedValue = seed.value;
    if (seedValue == "") {
        toastr.error("Specify a seed IP:PORT before continuing!");
        return;
    }
    seed.value = "";
    var all = $(".runStep").map(function() {
        this.innerText = this.innerText.replace(/ -h [^ ]+/," -h "+seedValue);
    });
    var all = $(".runStepClean").map(function() {
        this.innerText = this.innerText.replace(/ -h [^ ]+/," -h "+seedValue);
    });
    localStore();
    toastr.success("Seed address replaced!");
    $('#modal-replaceSeed').modal('hide');
}

function addAsbenchShow() {
    var item=document.getElementById("addAsbechLatencyPct");
    if ($("#addAsbechLatency").val() == "Yes") {
        item.disabled=false;
        item.style='';
    } else {
        item.disabled=true;
        item.value="";
        item.style="background-color: #b1b1b1";
    }
}

function addAsbench(isEdit=false) {
    var uuid = "";
    if (isEdit) {
        uuid = document.getElementById("asbenchEditUUID").value;
    }
    var title=document.getElementById("addAsbechTitle").value;
    if (title == "") {
        toastr.error("Specify a title/name of the script line!");
        return;
    }

    var runCount = document.getElementById("addAsbechQty").value;
    if (isNaN(runCount)) {
        toastr.error("Asbench run times qty must be a number!");
        return;
    }

    var aerolabClientName = document.getElementById("addAsbechAerolab").value;
    if (aerolabClientName == "") {
        aerolabClientName = "clientName";
    }

    var code = "asbench"
    var abort = false;
    var all = $(".asbench-switch-type").map(function() {
        var switchType = this.innerText;
        var data = $(this).parent().find(".asbench-switch-data")[0];
        var param = $(this).parent().children(".asbench-switch")[0];
        var required = false;
        if ($(this).parent().children(".asbench-switch-required").length > 0) {
            required = true;
        }
        if ((switchType == "string")||(switchType == "int")) {
            data = data.value;
            if (switchType == "int") {
                if (isNaN(data)) {
                    var label = $(this).parent().children("label")[0].innerText;
                    toastr.error(label+" is not a number!");
                    abort = true;
                    return;
                }        
            }
            if (data != "") {
                code = code + " " + param.innerText + data;
            } else {
                if (required) {
                    var label = $(this).parent().children("label")[0].innerText;
                    toastr.error(label+" is a required field!");
                    abort = true;
                    return;
                }
            }
        } else {
            data = data.innerText;
            if (param.value == "Yes") {
                code = code + " " + data;
            }
        }
    });

    if (abort) {
        return;
    }

    var clientList = document.getElementById("addAsbechClientsList").value;
    addEntry(title, code, runCount+";"+aerolabClientName+";"+clientList, true, uuid);
    document.getElementById("addAsbechTitle").value = "";
    $('#modal-asbench').modal('hide');
}

function workloadRelabel(item) {
    var nVal = "";
    for (const child of item.children) {
        if (child.nodeName == "SELECT") {
            nVal = child.value;
        }
    }
    for (const child of item.parentNode.children) {
        if (child.className.includes("workloadPct")) {
            if ((nVal == "I")||(nVal == "DB")||(nVal == "")) {
                document.getElementById("addAsbechRuntime").value = "0";
                //child.children[0].style="display:none";
                child.children[0].children[1].disabled=true;
                child.children[0].children[1].style="background-color: #b1b1b1";
                child.children[0].children[1].placeholder="";
                child.children[0].children[0].innerText="Percentage spread for workload";
            } else {
                if ((document.getElementById("addAsbechRuntime").value == "0")||(document.getElementById("addAsbechRuntime").value == "")) {
                    document.getElementById("addAsbechRuntime").value = "10";
                }
                //child.children[0].style="";
                child.children[0].children[1].disabled=false;
                child.children[0].children[1].style='';
                for (const childx of child.children[0].children) {
                    if (childx.nodeName == "LABEL") {
                        if ((nVal == "RU")||(nVal == "RR")) {
                            childx.innerText = "Percentage of the load to use as read";
                        } else if ((nVal == "RUD")||(nVal == "RUF")) {
                            childx.innerText = "Percentages of load to use as read and update";
                        }
                    } else {
                        if ((nVal == "RU")||(nVal == "RR")) {
                            childx.placeholder = "ex using 80% read and 20% write: 80";
                            colorController(childx);
                        } else if ((nVal == "RUD")||(nVal == "RUF")) {
                            childx.placeholder = "ex using 50% read and 30% update: 50,30";
                            colorController(childx);
                        }
                        childx.value = '';
                    }
                }
            }
            return;
        }
      }
}

function workloadUpdate() {
    var myParent = $(document).find("#asbenchWorkloads")[0];
    var data = $(myParent).children(".asbench-switch-data")[0];
    data.value = "";
    var err = false;
    var all = $(myParent).find(".workloadsSelect").map(function() {
        if (err) {
            return;
        }
        if (this.value == "") {
            return;
        }
        var res = "";
        if (data.value == "") {
            res = this.value;
        } else {
            res = data.value + "," + this.value;
        }
        if ((this.value != "I")&&(this.value != "DB")) {
            var spread = $(this).parent().parent().find(".workloadSpread")[0];
            res = res + "," + spread.value;
            if (spread.value != "") {
                data.value = res;
            } else {
                data.value = "";
                err = true;
            }
        } else {
            data.value = res;
        }
    });
}

function objectRelabel(item) {
    var nVal = "";
    for (const child of item.children) {
        if (child.nodeName == "SELECT") {
            nVal = child.value;
        }
    }
    for (const child of item.parentNode.children) {
        if (child.className.includes("objectPct")) {
            if ((nVal == "b")||(nVal == "D")||(nVal == "")) {
                child.children[0].style="display:none";
            } else {
                child.children[0].style="";
                for (const childx of child.children[0].children) {
                    if (childx.nodeName == "LABEL") {
                        if (nVal == "I") {
                            childx.innerText = "Number of randomized bits in int";
                        } else if (nVal == "S") {
                            childx.innerText = "Size of string (number of bytes)";
                        } else if (nVal == "B") {
                            childx.innerText = "Size of binary data (number of bytes)";
                        }
                    } else {
                        if (nVal == "I") {
                            childx.placeholder = "ex: 1, ex: 5";
                        } else if (nVal == "S") {
                            childx.placeholder = "ex 4kb: 4096";
                        } else if (nVal == "B") {
                            childx.placeholder = "ex 16kb: 16384";
                        }
                        childx.value = '';
                    }
                }
            }
            return;
        }
      }
}

function objectUpdate() {
    var myParent = $(document).find("#asbenchObjects")[0];
    var data = $(myParent).find(".asbench-switch-data")[0];
    data.value = "";
    var err = false;
    var all = $(myParent).find(".objectsSelect").map(function() {
        if (err) {
            return;
        }
        colorController(this);
        if (this.value == "") {
            return;
        }
        var res = "";
        var count = $(this).parent().parent().find(".objectCount")[0].value;
        if (data.value == "") {
            if ((count != "")&&(count > 1)) {
                res = count + "*" + this.value;
            } else {
                res = this.value;
            }
        } else {
            if ((count != "")&&(count > 1)) {
                res = data.value + "," + count + "*" + this.value;
            } else {
                res = data.value + "," + this.value;
            }
        }
        if ((this.value != "b")&&(this.value != "D")) {
            var spread = $(this).parent().parent().find(".objectSpread")[0];
            colorController(spread);
            var sprea = spread.value;
            if (spread.value == "") {
                sprea = "0";
            }
            res = res + sprea;
            if (sprea != "") {
                data.value = res;
            } else {
                data.value = "";
                err = true;
            }
        } else {
            data.value = res;
        }
    });
    if (data.value != "") {
        data.value = "'" + data.value + "'";
    }
}

function addWait(isEdit=false) {
    var uuid = "";
    if (isEdit) {
        uuid = document.getElementById("waitEditUUID").value;
    }
    var title = document.getElementById("waitTitle").value;
    var aerolabClientName = document.getElementById("waitClientName").value;
    var clientList = document.getElementById("waitClients").value;
    var code = "RET=0; while [ ${RET} -eq 0 ]; do sleep 1; pidof asbench; RET=$?; done";
    addEntry(title, code, "0;"+aerolabClientName+";"+clientList, true, uuid);
    $('#modal-wait').modal('hide');
}

function addSleep(isEdit=false) {
    var uuid = "";
    if (isEdit) {
        uuid = document.getElementById("sleepEditUUID").value;
    }
    var title = document.getElementById("sleepTitle").value;
    var sleepTime = document.getElementById("sleepTime").value;
    var code = "sleep " + sleepTime;
    addEntry(title, code, sleepTime + ";;", true, uuid);
    $('#modal-sleep').modal('hide');
}

function addKill(isEdit=false) {
    var uuid = "";
    if (isEdit) {
        uuid = document.getElementById("killEditUUID").value;
    }
    var title = document.getElementById("killTitle").value;
    var aerolabClientName = document.getElementById("killClientName").value;
    var clientList = document.getElementById("killClients").value;
    var code = "pkill -9 asbench";
    addEntry(title, code, "0;"+aerolabClientName+";"+clientList, true, uuid);
    $('#modal-kill').modal('hide');
}
