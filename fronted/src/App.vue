<!-- src/App.vue -->
<template>
  <div id="app">
    <h1>远程桌面控制</h1>
    <RemoteDesktop />
  </div>
</template>

<script>
import RemoteDesktop from './components/RemoteDesktop.vue'

export default {
  name: 'App',
  components: {
    RemoteDesktop
  }
}
</script>

<style>
#app {
  text-align: center;
  margin-top: 20px;
}
</style>