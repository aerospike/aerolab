# TODO items before release - WIP notes

new form type: upload file - tmp file upload and fill into form
 * some elements need this, like file upload, download, etc
home page, with config options
form submit handler (show command, or run command, show output, fill the actions, show success/fail/in-progress)
some elements in webui should not be forms but show as "not implemented in webui" - like webui, rest commands
support for handling special header - user/owner
implement console for all attach commands
implement inventory listing as another item, that is not a "command item" at the top, with separator under it, together with common config options item too ("special forms")
webconsole and/or login ssh script download from web UI (or maybe aerolab webUI will do the socket->node ssh?)
pendingActions modal - allow copy of path where log is and copy of output of aerolab command - log should also contain the yaml of the command executed
submit should do a jquery submit and present result, do NOT submit the actual page as all selections will be lost
add new tag: required - this should show a star next to required items, and JS should, on submit attempt, check if all required items are filled in
allow loading a historical command into the form from previous run/json?
--
implement handler for json/yaml command running:
cat command |aerolab fromjson
~/.aerolab/logs/user-id/timestamp-uuid.json
{
	"command": {
		...,
	},
	"log": [
		"",
		"",
		...
	],
}
-=-=-
TODO: agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
-=-=-=-
other HTML notes
pending-action-count: innerText
* classes: badge-warning - in progress; badge-danger - some actions failed; badge-success - all actions succeeded

pending-action-icon: classes: "far fa-bell" - nothing in progress; make spinny wheel if actions in progress exist

pendingActionShowAll(): under pending-user-action-list:
* cleanup
	* remove all items with class dropdown-divider
	* remove all items with class dropdown-item (and their sub-items)
* add this layout, use spinner icon for running actions, red X for failed, and green V for success
  <div class="dropdown-divider"></div>
  <a href="#" class="dropdown-item">
    <i class="fas fa-envelope mr-2"></i> 4 new messages
    <span class="float-right text-muted text-sm">3 mins</span>
    <br><span class="text-muted text-sm">Robert Glonek</span><span class="text-muted float-right text-sm">rglonek@aerospike.com</span>
  </a>
* each one of these should open a modal which will show the output of aerolab command that ran - reuse same modal, just use a different action ID
* display all running and last X finished runs (configurable on server side)
* footer should have "clear" button
* clear should only clear notifications for current user - i.e. server needs to keep the track on truncate of notifications (by finish time) per user
