<script lang="ts">
  import ChatApp from "../../../../ChatApp.svelte";

  let { params } = $props();
</script>

<ChatApp routeWorkspaceID={params.workspaceID} routeTargetID={params.targetID} />
