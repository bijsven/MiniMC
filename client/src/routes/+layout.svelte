<script lang="ts">
    import "../app.css";
    import favicon from "$lib/assets/favicon.png";

    let { children } = $props();
</script>

<svelte:head>
    <link rel="icon" href={favicon} />
</svelte:head>

<content class="bg-[#242424] h-full w-full absolute">
    {@render children?.()}
</content>
