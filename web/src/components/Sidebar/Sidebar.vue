<template>
  <aside class="w-64 h-full bg-gray-50 border-r border-gray-200 flex flex-col">
    <div class="p-4">
      <div class="relative">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path></svg>
        <input type="text" placeholder="Search" v-model="search" class="w-full pl-9 pr-4 py-2 rounded-md bg-white border border-gray-200 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500">
      </div>
    </div>

    <div class="flex-1 overflow-y-auto">
      <div v-for="(section, type) in sections" :key="type" class="px-4 py-2">
        <div class="flex items-center justify-between mb-2">
          <button @click="toggleCollapse(type)" class="flex items-center text-sm font-semibold text-gray-600 hover:text-primary w-full">
             <svg class="w-4 h-4 mr-2 transition-transform" :class="{ 'rotate-90': !collapsed[type] }" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path></svg>
            <span>{{ section.title }}</span>
          </button>
          <button @click="addItem(type)" class="text-gray-400 hover:text-primary">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path></svg>
          </button>
        </div>
        <div v-if="!collapsed[type]" class="pl-4">
          <div v-if="loading[type]" class="py-2 text-center text-gray-400">
             <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-900 mx-auto"></div>
          </div>
          <div v-else-if="error[type]" class="text-red-500 text-xs py-2">
            {{ error[type] }}
          </div>
          <div v-else class="space-y-1">
            <div v-for="item in filteredItems(type)" :key="item.id || item.name" 
                 class="flex items-center py-1.5 px-2 rounded-md hover:bg-gray-200 cursor-pointer group"
                 @click="$emit('select-item', { type, id: item.id || item.name })">
              <svg v-html="section.icon" class="w-4 h-4 mr-3 text-gray-500 group-hover:text-primary"></svg>
              <span class="text-sm text-gray-700 group-hover:text-primary">{{ item.id || item.name }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </aside>
</template>

<script>
import { hubApi } from '../../api/index.js';

export default {
  name: 'Sidebar',
  data() {
    return {
      search: '',
      items: {
        inputs: [],
        outputs: [],
        rulesets: [],
        plugins: [],
        projects: []
      },
      loading: {
        inputs: false,
        outputs: false,
        rulesets: false,
        plugins: false,
        projects: false
      },
      error: {
        inputs: null,
        outputs: null,
        rulesets: null,
        plugins: null,
        projects: null
      },
      collapsed: {
        inputs: false,
        outputs: true,
        rulesets: false,
        plugins: true,
        projects: true
      },
      sections: {
        inputs: { title: 'Input', icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>' },
        outputs: { title: 'Output', icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H3"></path>' },
        rulesets: { title: 'Ruleset', icon: '<path d="M4 6a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2V6z"></path><path d="M10 9h4"></path><path d="M10 13h4"></path><path d="M10 17h4"></path>' },
        plugins: { title: 'Plugin', icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 11V7a4 4 0 00-8 0v4M5 9h14l1 12H4L5 9z"></path>' },
        projects: { title: 'Project', icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"></path>' }
      }
    }
  },
  async created() {
    await this.fetchAllItems();
  },
  methods: {
    toggleCollapse(type) {
        this.collapsed[type] = !this.collapsed[type];
    },
    filteredItems(type) {
      if (!this.search) return this.items[type];
      const keyword = this.search.toLowerCase();
      return this.items[type].filter(item => {
        const id = (item.id || '').toLowerCase();
        const name = (item.name || '').toLowerCase();
        return id.includes(keyword) || name.includes(keyword);
      });
    },
    async fetchAllItems() {
      const types = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects'];
      await Promise.all(types.map(type => this.fetchItems(type)));
    },
    async fetchItems(type) {
      this.loading[type] = true;
      this.error[type] = null;
      try {
        const fetchMethod = `fetch${type.charAt(0).toUpperCase() + type.slice(1, -1)}s`;
        const response = await hubApi[fetchMethod]();
        this.items[type] = response;
      } catch (err) {
        this.error[type] = `Failed to load ${type}.`;
        console.error(`Error fetching ${type}:`, err);
      } finally {
        this.loading[type] = false;
      }
    },
    getDefaultConfig(type) {
      const timestamp = Date.now();
      const id = `new_${type.slice(0, -1)}_${timestamp}`;
      switch (type) {
        case 'inputs':
          return { id, raw: `name: "${id}"\ntype: "file"\nfile:\n  path: "/path/to/input.json"\n  format: "json"` };
        case 'outputs':
          return { id, raw: `name: "${id}"\ntype: "file"\nfile:\n  path: "/path/to/output.json"\n  format: "json"` };
        case 'rulesets':
          return { id, raw: `<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<root type=\"DETECTION\" />` };
        case 'projects':
          return { id, raw: `name: "${id}"\nflow:\n  - from: "input.default"\n    to: "ruleset.default"\n  - from: "ruleset.default"\n    to: "output.default"` };
        default:
          return { id: '', raw: '' };
      }
    },
    async addItem(type) {
        if (type === 'plugins') {
            // Handle plugin adding if necessary, maybe a modal?
            alert('Cannot add plugins via this interface.');
            return;
        }
      try {
        const config = this.getDefaultConfig(type);
        const createMethod = `create${type.charAt(0).toUpperCase() + type.slice(1, -1)}`;
        await hubApi[createMethod](config.id, config.raw);
        await this.fetchItems(type);
      } catch (err) {
        this.error[type] = `Failed to create ${type}.`;
        console.error(`Error creating ${type}:`, err);
      }
    }
  }
};
</script> 