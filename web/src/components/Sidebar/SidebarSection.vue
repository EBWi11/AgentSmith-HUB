<template>
  <div class="mb-4">
    <div class="flex items-center justify-between mb-2">
      <div class="flex items-center space-x-2 cursor-pointer" @click="toggleExpanded">
        <i :class="['fas', 'text-xs', 'text-gray-500', expanded ? 'fa-chevron-down' : 'fa-chevron-right']"></i>
        <span class="text-sm font-medium text-gray-700">{{ title }}</span>
      </div>
      <button class="text-gray-500 hover:text-gray-700 cursor-pointer whitespace-nowrap !rounded-button" @click="addNewItem">
        <i class="fas fa-plus"></i>
      </button>
    </div>

    <div v-if="expanded" class="pl-6">
      <div v-for="item in items" :key="item.name" class="mb-2">
        <div class="flex items-center py-1 px-2 hover:bg-gray-100 rounded cursor-pointer" @click="toggleItem(item)">
          <i :class="getItemIcon(item.type)" class="text-gray-500 mr-2 text-sm"></i>
          <span class="text-sm text-gray-700">{{ item.name }}</span>
        </div>
        <div v-if="item.expanded && item.items && item.items.length > 0" class="pl-6">
          <div v-for="subItem in item.items" :key="subItem" class="flex items-center py-1 px-2 hover:bg-gray-100 rounded cursor-pointer">
            <i :class="getSubItemIcon(item.type)" class="text-gray-500 mr-2 text-sm"></i>
            <span class="text-sm text-gray-700">{{ subItem }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'SidebarSection',
  props: {
    title: {
      type: String,
      required: true
    },
    items: {
      type: Array,
      required: true
    }
  },
  data() {
    return {
      expanded: true
    }
  },
  methods: {
    toggleExpanded() {
      this.expanded = !this.expanded;
    },
    toggleItem(item) {
      item.expanded = !item.expanded;
    },
    addNewItem() {
      const newItem = {
        name: `new_${this.title.toLowerCase()}_${this.items.length + 1}`,
        type: this.title.toLowerCase(),
        expanded: false,
        items: []
      };
      this.items.push(newItem);
    },
    getItemIcon(type) {
      const icons = {
        database: 'fas fa-database',
        output: 'fas fa-terminal',
        ruleset: 'fas fa-book',
        plugin: 'fas fa-plug',
        project: 'fas fa-cog'
      };
      return icons[type] || 'fas fa-file';
    },
    getSubItemIcon(type) {
      const icons = {
        database: 'fas fa-table',
        output: 'fas fa-file-code',
        ruleset: 'fas fa-file-alt',
        plugin: 'fas fa-puzzle-piece',
        project: 'fas fa-wrench'
      };
      return icons[type] || 'fas fa-file';
    }
  }
}
</script> 