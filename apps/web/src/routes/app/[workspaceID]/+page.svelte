<script lang="ts">
  import ChatApp from "../../../ChatApp.svelte";

  let { params } = $props();
</script>

<ChatApp routeWorkspaceID={params.workspaceID} />
