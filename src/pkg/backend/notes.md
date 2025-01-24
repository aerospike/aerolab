* after any action, like instancesAddTags(), it's up to the caller to call ForceRefreshInventory if they feel inclined to do so
  * eg if the state cache is enabled, or if you want to perform further actions/jobs on the cluster
