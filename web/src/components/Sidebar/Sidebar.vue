<template>
  <aside class="h-full bg-white shadow-sm flex flex-col font-sans transition-all duration-300"
         :class="props.collapsed ? 'w-16' : 'w-72'"
         :style="props.collapsed ? 'min-width: 64px' : 'min-width: 288px'"
         data-component="sidebar"
         ref="sidebarRef">
    <!-- Header with toggle button -->
    <div class="flex items-center justify-between px-3 pt-5 pb-3">
      <div v-if="!props.collapsed" class="flex items-center flex-1 pl-6">
        <router-link 
          to="/app" 
          class="text-lg font-bold text-gray-900 truncate hover:text-blue-600 transition-colors duration-200 cursor-pointer select-none"
          title="Back to Dashboard"
        >
          AgentSmith-HUB
        </router-link>
      </div>

      <div class="relative" :class="props.collapsed ? 'mx-auto' : 'mr-3'">
        <button 
          @click="emit('toggle-collapse')"
          class="p-1 rounded-full hover:bg-gray-100 transition-colors flex items-center justify-center w-6 h-6"
          :title="props.collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        >
          <svg class="w-4 h-4 transition-transform duration-200"
               :class="{ 'rotate-180': props.collapsed }"
               fill="none" 
               stroke="currentColor" 
               viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 19l-7-7 7-7m8 14l-7-7 7-7"/>
          </svg>
        </button>
      </div>
    </div>
    
    <!-- Content wrapper -->
    <div class="flex-1 flex flex-col min-h-0" :class="props.collapsed ? 'px-2' : 'px-3'">
      <!-- Search bar - only show when not collapsed -->
      <div v-if="!props.collapsed" class="mb-4">
        <div class="relative">
          <input
            type="text"
            placeholder="Search"
            v-model="search"
            @input="debouncedSearch(search)"
            class="w-full pl-7 pr-8 py-1.5 rounded-lg bg-gray-50 border border-gray-100 text-sm focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition"
          />
          <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path></svg>
          <!-- Search loading indicator -->
          <div v-if="searchLoading" class="absolute right-2.5 top-1/2 -translate-y-1/2">
            <div class="w-3 h-3 border border-gray-400 border-t-transparent rounded-full animate-spin"></div>
          </div>
        </div>
      </div>
      <!-- Navigation -->
      <div class="flex-1 min-h-0 overflow-y-auto custom-scrollbar">
        <div v-for="(section, type) in sections" :key="type" class="mb-4">
        <!-- Regular sections -->
        <div>
          <div class="flex items-center justify-between mb-1.5">
            <button
              @click="toggleCollapse(type)"
              class="text-[13px] font-bold text-gray-900 tracking-wide uppercase focus:outline-none group"
              :class="props.collapsed ? 'w-full flex justify-center' : 'flex items-center'"
              :title="props.collapsed ? section.title : ''"
              style="min-width:0;"
            >
              <!-- Collapsed view: just icon centered -->
              <template v-if="props.collapsed">
                <div class="w-8 h-8 flex items-center justify-center mx-auto text-gray-600" v-html="section.icon"></div>
              </template>
              <!-- Expanded view: normal layout -->
              <template v-else>
                <div class="flex items-center w-full">
                  <svg
                    class="w-4 h-4 mr-1.5 transition-transform duration-200 flex-shrink-0"
                    :class="{ 'rotate-90': !collapsed[type] }"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                  </svg>
                  <!-- Add section icon -->
                  <div class="w-4 h-4 mr-1.5 text-gray-600 flex-shrink-0" v-html="section.icon"></div>
                  <span class="truncate">{{ section.title }}</span>
                </div>
              </template>
            </button>
            <div v-if="!props.collapsed" class="relative mr-3">
              <button v-if="!section.children" @click="openAddModal(type)" class="p-1 rounded-full hover:bg-primary/10 text-primary transition flex items-center justify-center w-6 h-6">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path></svg>
              </button>
            </div>
          </div>
          <div v-if="!collapsed[type] && !props.collapsed" class="space-y-0.5">
            <div v-if="section.children" class="relative">
              <!-- 已经将Push Changes整合到section.children中，不再需要单独的部分 -->
              <div v-for="(child, index) in section.children" :key="child.type"
                   class="relative flex items-center justify-between py-1 rounded-md group cursor-pointer transition-all hover:bg-gray-100"
                   :class="{ 'bg-blue-50': selected && selected.type === child.type }"
                   @click="$emit('select-item', { type: child.type })">
                <!-- Tree lines for children -->
                <!-- Vertical line connecting to next item (except for last item) -->
                <div class="absolute left-5 top-1/2 bottom-0 w-px bg-gray-300" v-if="index < section.children.length - 1"></div>
                <!-- Horizontal line to item -->
                <div class="absolute left-5 top-1/2 w-2 h-px bg-gray-300"></div>
                <!-- Vertical line from top to current item (all items have this) -->
                <div class="absolute left-5 top-0 h-1/2 w-px bg-gray-300"></div>
                
                <div class="flex items-center justify-between min-w-0 flex-1 pl-8 pr-3">
                  <!-- 移除所有子组件的图标 -->
                  <span class="text-sm truncate">{{ child.title }}</span>
                  <!-- Settings badges -->
                  <div v-if="settingsBadges[child.type] > 0" 
                       class="flex items-center justify-center min-w-[20px] h-5 px-1.5 text-xs font-medium rounded-full text-white ml-2"
                       :class="{
                         'bg-orange-500': child.type === 'pending-changes',
                         'bg-purple-500': child.type === 'load-local-components', 
                         'bg-red-500': child.type === 'error-logs'
                       }">
                    {{ settingsBadges[child.type] > 99 ? '99+' : settingsBadges[child.type] }}
                  </div>
                </div>
              </div>
            </div>
            <div v-else-if="!loading[type] && !error[type]" class="relative">
              <!-- Empty state: show only short vertical line when no components -->
              <div v-if="filteredItems(type).length === 0" class="relative py-1">
                <!-- Short vertical line for empty state -->
                <div class="absolute left-5 top-0 h-3 w-px bg-gray-300"></div>
              </div>
                              <!-- Special handling for plugins with built-in submenu -->
                <template v-else-if="type === 'plugins'">
                  <div v-if="getOrganizedPlugins().builtinPlugins.length > 0">
                  <!-- Built-in plugins submenu -->
                  <div class="relative">
                    <div class="relative flex items-center justify-between py-1 hover:bg-gray-100 rounded-md cursor-pointer group"
                         @click="collapsed.builtinPlugins = !collapsed.builtinPlugins">
                      <!-- Tree lines -->
                      <div class="absolute left-5 top-1/2 bottom-0 w-px bg-gray-300" v-if="getOrganizedPlugins().customPlugins.length > 0"></div>
                      <div class="absolute left-5 top-1/2 w-2 h-px bg-gray-300"></div>
                      <div class="absolute left-5 top-0 h-1/2 w-px bg-gray-300"></div>
                      
                      <div class="flex items-center min-w-0 flex-1 pl-8 pr-3">
                        <svg class="w-3 h-3 mr-2 transition-transform duration-200 flex-shrink-0"
                             :class="{ 'rotate-90': !collapsed.builtinPlugins }"
                             fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                        </svg>
                        <span class="text-sm text-gray-600 font-medium">Built-in Plugins</span>
                        <span class="ml-2 text-xs bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded-full">
                          {{ getOrganizedPlugins().builtinPlugins.length }}
                        </span>
                      </div>
                    </div>
                    
                                         <!-- Built-in plugins list -->
                     <div v-if="!collapsed.builtinPlugins" class="relative">
                       <!-- Continuous vertical line for built-in plugins section -->
                       <div class="absolute left-5 top-0 w-px bg-gray-300 z-10" 
                            :style="{ height: getOrganizedPlugins().customPlugins.length > 0 ? '100%' : (getOrganizedPlugins().builtinPlugins.length * 32) + 'px' }"></div>
                       
                       <div v-for="(item, index) in getOrganizedPlugins().builtinPlugins" :key="item.id"
                            class="relative flex items-center justify-between py-1 hover:bg-gray-100 rounded-md cursor-pointer group"
                            :class="{ 'bg-blue-50': selected && selected.id === item.id && selected.type === type }"
                            @click="handleItemClick(type, item)"
                            @dblclick="handleItemDoubleClick(type, item)">
                         <!-- Tree lines for built-in plugins -->
                         <div class="absolute left-8 top-1/2 bottom-0 w-px bg-gray-300 z-10" v-if="index < getOrganizedPlugins().builtinPlugins.length - 1"></div>
                         <div class="absolute left-8 top-1/2 w-2 h-px bg-gray-300 z-10"></div>
                         <div class="absolute left-8 top-0 h-1/2 w-px bg-gray-300 z-10"></div>
                        
                        <div class="flex items-center min-w-0 flex-1 pl-11 pr-3">
                          <div class="flex-1 min-w-0">
                            <div class="flex items-center">
                              <span class="text-sm truncate">{{ item.id }}</span>
                              <!-- Search match indicators -->
                              <div v-if="item.searchMatch" class="ml-1 flex items-center space-x-1">
                                <span v-if="item.searchMatch.nameMatch" 
                                      class="text-xs bg-blue-100 text-blue-700 px-1.5 py-0.5 rounded-full cursor-help"
                                      @mouseenter="showTooltip($event, 'Name match')"
                                      @mouseleave="hideTooltip">
                                  N
                                </span>
                                <span v-if="item.searchMatch.type === 'content' || item.searchMatch.type === 'both'" 
                                      class="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded-full cursor-help"
                                      @mouseenter="showTooltip($event, `Content match: Line ${item.searchMatch.lineNumber}`)"
                                      @mouseleave="hideTooltip">
                                  C
                                </span>
                              </div>
                            </div>
                            <!-- Show matching content line for content matches -->
                            <div v-if="item.searchMatch && (item.searchMatch.type === 'content' || item.searchMatch.type === 'both') && item.searchMatch.lineContent" 
                                 class="text-xs text-gray-500 truncate mt-0.5">
                              Line {{ item.searchMatch.lineNumber }}: {{ item.searchMatch.lineContent }}
                            </div>
                                                     </div>
                           <!-- Plugin function type badge -->
                           <span v-if="isCheckNodeType(item)" 
                                 class="ml-2 text-xs bg-green-100 text-green-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                                 @mouseenter="showTooltip($event, 'Check Node Plugin')"
                                 @mouseleave="hideTooltip">
                             C
                           </span>
                           <span v-else-if="isPluginNodeType(item)" 
                                 class="ml-2 text-xs bg-purple-100 text-purple-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                                 @mouseenter="showTooltip($event, 'Plugin Node')"
                                 @mouseleave="hideTooltip">
                             P
                           </span>
                         </div>
                        
                        <!-- Actions menu for built-in plugins -->
                        <div class="relative mr-3">
                          <button class="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-200 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity menu-toggle-button w-6 h-6 flex items-center justify-center"
                                  @click.stop="toggleMenu(item)">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"></path>
                            </svg>
                          </button>
                          <!-- Dropdown menu for built-in plugins -->
                          <div v-if="item.menuOpen" 
                               class="absolute right-0 mt-1 w-48 bg-white rounded-md shadow-lg z-10 dropdown-menu"
                               @click.stop>
                            <div class="py-1">
                              <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                                 @click.prevent.stop="openTestPlugin(item)">
                                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                                </svg>
                                Test Plugin
                              </a>
                              
                              <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                                 @click.prevent.stop="openPluginStatsModal(item)">
                                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                                </svg>
                                View Stats
                              </a>
                              
                              <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                                 @click.prevent.stop="copyName(item)">
                                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"></path>
                                </svg>
                                Copy Name
                              </a>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                
                <!-- Custom plugins -->
                <div v-for="(item, index) in getOrganizedPlugins().customPlugins" :key="item.id || item.name"
                     class="relative flex items-center justify-between py-1 hover:bg-gray-100 rounded-md cursor-pointer group"
                     :class="{ 'bg-blue-50': selected && (selected.id === item.id || selected.id === item.name) && selected.type === type }"
                     @click="handleItemClick(type, item)"
                     @dblclick="handleItemDoubleClick(type, item)">
                  <!-- Tree lines for custom plugins -->
                  <div class="absolute left-5 top-1/2 bottom-0 w-px bg-gray-300" v-if="index < getOrganizedPlugins().customPlugins.length - 1"></div>
                  <div class="absolute left-5 top-1/2 w-2 h-px bg-gray-300"></div>
                  <div class="absolute left-5 top-0 h-1/2 w-px bg-gray-300"></div>
                  
                                     <div class="flex items-center min-w-0 flex-1 pl-8 pr-3">
                     <div class="flex-1 min-w-0">
                       <div class="flex items-center">
                         <span class="text-sm truncate">{{ item.id || item.name }}</span>
                         <!-- Search match indicators -->
                         <div v-if="item.searchMatch" class="ml-1 flex items-center space-x-1">
                           <span v-if="item.searchMatch.nameMatch" 
                                 class="text-xs bg-blue-100 text-blue-700 px-1.5 py-0.5 rounded-full cursor-help"
                                 @mouseenter="showTooltip($event, 'Name match')"
                                 @mouseleave="hideTooltip">
                             N
                           </span>
                           <span v-if="item.searchMatch.type === 'content' || item.searchMatch.type === 'both'" 
                                 class="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded-full cursor-help"
                                 @mouseenter="showTooltip($event, `Content match: Line ${item.searchMatch.lineNumber}`)"
                                 @mouseleave="hideTooltip">
                             C
                           </span>
                         </div>
                       </div>
                       <!-- Show matching content line for content matches -->
                       <div v-if="item.searchMatch && (item.searchMatch.type === 'content' || item.searchMatch.type === 'both') && item.searchMatch.lineContent" 
                            class="text-xs text-gray-500 truncate mt-0.5">
                         Line {{ item.searchMatch.lineNumber }}: {{ item.searchMatch.lineContent }}
                       </div>
                     </div>
                     <!-- Plugin function type badge -->
                     <span v-if="isCheckNodeType(item)" 
                           class="ml-2 text-xs bg-green-100 text-green-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                           @mouseenter="showTooltip($event, 'Check Node Plugin')"
                           @mouseleave="hideTooltip">
                       C
                     </span>
                     <span v-else-if="isPluginNodeType(item)" 
                           class="ml-2 text-xs bg-purple-100 text-purple-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                           @mouseenter="showTooltip($event, 'Plugin Node')"
                           @mouseleave="hideTooltip">
                       P
                     </span>
                     <!-- Temporary file badge -->
                     <span v-if="item.hasTemp" 
                           class="ml-2 text-xs bg-blue-100 text-blue-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                           @mouseenter="showTooltip($event, 'Temporary Version')"
                           @mouseleave="hideTooltip">
                       T
                     </span>
                   </div>
                  
                  <!-- Actions menu for custom plugins -->
                  <div class="relative mr-3">
                    <button class="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-200 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity menu-toggle-button w-6 h-6 flex items-center justify-center"
                            @click.stop="toggleMenu(item)">
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"></path>
                      </svg>
                    </button>
                    <!-- Dropdown menu for custom plugins -->
                    <div v-if="item.menuOpen" 
                         class="absolute right-0 mt-1 w-48 bg-white rounded-md shadow-lg z-10 dropdown-menu"
                         @click.stop>
                      <div class="py-1">
                        <!-- Edit action -->
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="closeAllMenus(); $emit('open-editor', { type, id: item.id, isEdit: true })">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
                          </svg>
                          Edit
                        </a>
                        
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="openTestPlugin(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                          </svg>
                          Test Plugin
                        </a>
                        
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="openPluginStatsModal(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                          </svg>
                          View Stats
                        </a>
                        
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="copyName(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"></path>
                          </svg>
                          Copy Name
                        </a>
                        
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="openPluginUsageModal(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          View Usage
                        </a>
                        
                        <!-- Delete action -->
                        <div class="border-t border-gray-100 my-1"></div>
                        <a href="#" class="flex items-center px-4 py-2 text-sm text-red-600 hover:bg-red-50" 
                           @click.prevent.stop="openDeleteModal(type, item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                          Delete
                        </a>
                      </div>
                    </div>
                  </div>
                </div>
              </template>
              <!-- Normal component list for non-plugin types -->
              <div v-else v-for="(item, index) in filteredItems(type)" :key="item.id" 
                   class="relative flex items-center justify-between py-1 hover:bg-gray-100 rounded-md cursor-pointer group"
                   :class="{ 'bg-blue-50': selected && selected.id === item.id && selected.type === type }"
                   @click="handleItemClick(type, item)"
                   @dblclick="handleItemDoubleClick(type, item)">
                <!-- Tree lines -->
                <!-- Vertical line connecting to next item (except for last item) -->
                <div class="absolute left-5 top-1/2 bottom-0 w-px bg-gray-300" v-if="index < filteredItems(type).length - 1"></div>
                <!-- Horizontal line to item -->
                <div class="absolute left-5 top-1/2 w-2 h-px bg-gray-300"></div>
                <!-- Vertical line from top to current item (all items have this) -->
                <div class="absolute left-5 top-0 h-1/2 w-px bg-gray-300"></div>
                
                <div class="flex items-center min-w-0 flex-1 pl-8 pr-3">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center">
                      <span class="text-sm truncate">{{ item.id }}</span>
                      <!-- Search match indicators -->
                      <div v-if="item.searchMatch" class="ml-1 flex items-center space-x-1">
                        <span v-if="item.searchMatch.nameMatch" 
                              class="text-xs bg-blue-100 text-blue-700 px-1.5 py-0.5 rounded-full cursor-help"
                              @mouseenter="showTooltip($event, 'Name match')"
                              @mouseleave="hideTooltip">
                          N
                        </span>
                        <span v-if="item.searchMatch.type === 'content' || item.searchMatch.type === 'both'" 
                              class="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded-full cursor-help"
                              @mouseenter="showTooltip($event, `Content match: Line ${item.searchMatch.lineNumber}`)"
                              @mouseleave="hideTooltip">
                          C
                        </span>
                      </div>
                    </div>
                    <!-- Show matching content line for content matches -->
                    <div v-if="item.searchMatch && (item.searchMatch.type === 'content' || item.searchMatch.type === 'both') && item.searchMatch.lineContent" 
                         class="text-xs text-gray-500 truncate mt-0.5">
                      Line {{ item.searchMatch.lineNumber }}: {{ item.searchMatch.lineContent }}
                    </div>
                  </div>
                  <!-- Plugin function type badge for plugins -->
                  <span v-if="type === 'plugins' && isCheckNodeType(item)" 
                        class="ml-2 text-xs bg-green-100 text-green-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                        @mouseenter="showTooltip($event, 'Check Node Plugin')"
                        @mouseleave="hideTooltip">
                    C
                  </span>
                  <span v-else-if="type === 'plugins' && isPluginNodeType(item)" 
                        class="ml-2 text-xs bg-purple-100 text-purple-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                        @mouseenter="showTooltip($event, 'Plugin Node')"
                        @mouseleave="hideTooltip">
                    P
                  </span>

                  <!-- Input type badge for inputs -->
                  <span v-if="type === 'inputs' && getInputTypeInfo(item)" 
                        class="ml-2 text-xs w-auto min-w-[20px] h-5 px-1 flex items-center justify-center rounded-full cursor-help"
                        :class="getInputTypeInfo(item).color"
                        @mouseenter="showTooltip($event, getInputTypeInfo(item).tooltip)"
                        @mouseleave="hideTooltip">
                    {{ getInputTypeInfo(item).icon }}
                  </span>
                  
                  <!-- Output type badge for outputs -->
                  <span v-if="type === 'outputs' && getOutputTypeInfo(item)" 
                        class="ml-2 text-xs w-auto min-w-[20px] h-5 px-1 flex items-center justify-center rounded-full cursor-help"
                        :class="getOutputTypeInfo(item).color"
                        @mouseenter="showTooltip($event, getOutputTypeInfo(item).tooltip)"
                        @mouseleave="hideTooltip">
                    {{ getOutputTypeInfo(item).icon }}
                  </span>

                  <!-- Ruleset type badge for rulesets -->
                  <span v-if="type === 'rulesets' && getRulesetTypeInfo(item)" 
                        class="ml-2 text-xs w-auto min-w-[20px] h-5 px-1 flex items-center justify-center rounded-full cursor-help"
                        :class="getRulesetTypeInfo(item).color"
                        @mouseenter="showTooltip($event, getRulesetTypeInfo(item).tooltip)"
                        @mouseleave="hideTooltip">
                    {{ getRulesetTypeInfo(item).icon }}
                  </span>

                  <!-- Temporary file badge -->
                  <span v-if="item.hasTemp" 
                        class="ml-2 text-xs bg-blue-100 text-blue-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                        @mouseenter="showTooltip($event, 'Temporary Version')"
                        @mouseleave="hideTooltip">
                    T
                  </span>
                  <!-- Cluster inconsistency warning -->
                  <span v-if="type === 'projects' && hasClusterInconsistency(item.id)" 
                        class="ml-2 w-3 h-3 bg-yellow-400 rounded-full cursor-help animate-pulse"
                        @mouseenter="showTooltip($event, 'Cluster status inconsistency detected')"
                        @mouseleave="hideTooltip">
                  </span>
                  
                  <!-- Project status badge -->
                  <span v-if="type === 'projects' && item.status" 
                        class="ml-2 text-xs w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                        :class="{
                          'bg-green-100 text-green-800': item.status === 'running',
                          'bg-gray-100 text-gray-800': item.status === 'stopped',
                          'bg-blue-100 text-blue-800 animate-pulse': item.status === 'starting',
                          'bg-orange-100 text-orange-800 animate-pulse': item.status === 'stopping',
                          'bg-red-100 text-red-800': item.status === 'error'
                        }"
                        @mouseenter="showTooltip($event, getStatusTitle(item))"
                        @mouseleave="hideTooltip">
                    {{ getStatusLabel(item.status) }}
                  </span>
                </div>
                
                <!-- Actions menu -->
                <div class="relative mr-3">
                  <button class="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-200 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity menu-toggle-button w-6 h-6 flex items-center justify-center"
                          @click.stop="toggleMenu(item)">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"></path>
                    </svg>
                  </button>
                  <!-- Dropdown menu -->
                  <div v-if="item.menuOpen" 
                       class="absolute right-0 mt-1 w-48 bg-white rounded-md shadow-lg z-10 dropdown-menu"
                       @click.stop>
                    <div class="py-1">
                      <!-- Edit action -->
                      <a v-if="!(type === 'plugins' && item.type === 'local')" 
                         href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="closeAllMenus(); $emit('open-editor', { type, id: item.id, isEdit: true })">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
                        </svg>
                        Edit
                      </a>
                      
                      <!-- Project specific actions -->
                      <template v-if="type === 'projects'">
                        <!-- Start action -->
                        <a v-if="(item.status === 'stopped' || item.status === 'error') && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="startProject(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          Start
                        </a>
                        
                        <!-- Stop action -->
                        <a v-if="item.status === 'running' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="stopProject(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
                          </svg>
                          Stop
                        </a>
                        
                        <!-- Starting status display -->
                        <div v-if="item.status === 'starting'" class="flex items-center px-4 py-2 text-sm text-blue-600">
                          <div class="w-3 h-3 rounded-full bg-current animate-pulse mr-2"></div>
                          Starting...
                        </div>
                        
                        <!-- Stopping status display -->
                        <div v-if="item.status === 'stopping'" class="flex items-center px-4 py-2 text-sm text-orange-600">
                          <div class="w-3 h-3 rounded-full bg-current animate-pulse mr-2"></div>
                          Stopping...
                        </div>
                        
                        <!-- Restart action -->
                        <a v-if="(item.status === 'running' || item.status === 'error') && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                           @click.prevent.stop="restartProject(item)">
                          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                          </svg>
                          Restart
                        </a>
                      </template>
                      
                      <!-- Test actions for different component types -->
                      <a v-if="shouldShowConnectCheck(type, item)" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="checkConnection(type, item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                        </svg>
                        Connect Check
                      </a>
                      
                      <!-- View Sample Data for inputs (only for saved components) -->
                      <a v-if="type === 'inputs' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openSampleDataModal(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        </svg>
                        View Sample Data
                      </a>
                      
                      <!-- View Sample Data for rulesets (only for saved components) -->
                      <a v-if="type === 'rulesets' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openSampleDataModal(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        </svg>
                        View Sample Data
                      </a>
                      
                      <!-- View Sample Data for outputs (only for saved components) -->
                      <a v-if="type === 'outputs' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openSampleDataModal(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        </svg>
                        View Sample Data
                      </a>
                      
                      <!-- 添加查看使用情况选项，仅对已保存的input、output和ruleset类型显示 -->
                      <a v-if="(type === 'inputs' || type === 'outputs' || type === 'rulesets') && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openUsageModal(type, item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                        </svg>
                        View Usage
                      </a>
                      
                      <a v-if="type === 'plugins'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openTestPlugin(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                        </svg>
                        Test Plugin
                      </a>
                      
                      <a v-if="type === 'plugins'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openPluginStatsModal(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                        </svg>
                        View Stats
                      </a>
                      
                      <a v-if="type === 'rulesets'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openTestRuleset(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                        </svg>
                        Test Ruleset
                      </a>
                      
                      <a v-if="type === 'outputs'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openTestOutput(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                        </svg>
                        Test Output
                      </a>
                      
                      <a v-if="type === 'projects'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openTestProject(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                        </svg>
                        Test Project
                      </a>
                      
                      <!-- Cluster Status action for projects -->
                      <a v-if="type === 'projects'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="openClusterStatusModal(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
                        </svg>
                        Cluster Status
                      </a>
                      
                      <!-- Copy name action -->
                      <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="copyName(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"></path>
                        </svg>
                        Copy Name
                      </a>
                      
                      <!-- Delete action -->
                      <div v-if="!(type === 'plugins' && item.type === 'local')" class="border-t border-gray-100 my-1"></div>
                      <a v-if="!(type === 'plugins' && item.type === 'local')" 
                         href="#" class="flex items-center px-4 py-2 text-sm text-red-600 hover:bg-red-50" 
                         @click.prevent.stop="openDeleteModal(type, item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                        Delete
                      </a>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="loading[type]" class="py-1 text-center text-gray-400">
              <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-900 mx-auto"></div>
            </div>
            <div v-else-if="error[type]" class="text-red-500 text-xs py-1">
              {{ error[type] }}
            </div>
          </div>
        </div>
                      </div>
      </div>
    </div>

    <!-- Create New Modal -->
    <div v-if="showAddModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-4">Add {{ addType ? addType.slice(0, -1) : 'Component' }}</h3>
        <div class="mb-4">
          <label class="block text-sm font-medium text-gray-700 mb-1">Name</label>
          <input 
            type="text" 
            v-model="addName" 
            @keyup.enter="confirmAddName"
            class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500" 
            placeholder="Enter name" 
            ref="addNameInput"
          />
        </div>
        <div class="flex justify-end space-x-3">
          <button @click="closeAddModal" class="btn btn-secondary btn-sm">Cancel</button>
          <button 
            @click="confirmAddName" 
            :disabled="!addName || !addName.trim()"
            class="btn btn-primary btn-sm"
          >
            Create
          </button>
        </div>
        <div v-if="addError" class="mt-3 text-sm text-red-500">{{ addError }}</div>
      </div>
    </div>

    <!-- Connection Modal -->
    <div v-if="showConnectionModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-[800px] max-w-4xl max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Client Connection Status</h3>
          <button @click="closeConnectionModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div v-if="connectionLoading" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        
        <div v-else-if="connectionError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div class="ml-3">
              <p class="text-sm text-red-700">{{ connectionError }}</p>
            </div>
          </div>
        </div>
        
        <div v-else-if="connectionResult">
          <!-- Status Badge -->
          <div class="mb-4 flex items-center">
            <div class="px-2.5 py-0.5 rounded-full text-xs font-medium"
                 :class="{
                   'bg-green-100 text-green-800': connectionResult.status === 'success',
                   'bg-yellow-100 text-yellow-800': connectionResult.status === 'warning',
                   'bg-red-100 text-red-800': connectionResult.status === 'error'
                 }">
              {{ connectionResult.status }}
            </div>
            <span class="ml-2 text-sm text-gray-600">{{ connectionResult.message }}</span>
          </div>
          
          <!-- Client Type -->
          <div v-if="connectionResult.details.client_type" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Client Type:</h4>
            <div class="p-2 bg-gray-50 rounded-md text-sm">
              {{ connectionResult.details.client_type }}
            </div>
          </div>
          
          <!-- Connection Status -->
          <div v-if="connectionResult.details.connection_status" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Status:</h4>
            <div class="flex items-center p-2 border rounded-md"
                 :class="{
                   'border-green-200 bg-green-50': ['active', 'connected', 'always_connected'].includes(connectionResult.details.connection_status),
                   'border-yellow-200 bg-yellow-50': connectionResult.details.connection_status === 'idle',
                   'border-red-200 bg-red-50': ['not_configured', 'unsupported'].includes(connectionResult.details.connection_status)
                 }">
              <span class="w-2 h-2 rounded-full mr-2" 
                    :class="{
                      'bg-green-500': ['active', 'connected', 'always_connected'].includes(connectionResult.details.connection_status),
                      'bg-yellow-500': connectionResult.details.connection_status === 'idle',
                      'bg-red-500': ['not_configured', 'unsupported'].includes(connectionResult.details.connection_status),
                      'bg-gray-400': connectionResult.details.connection_status === 'unknown'
                    }"></span>
              <span class="text-sm">{{ connectionResult.details.connection_status }}</span>
            </div>
          </div>
          
          <!-- Connection Info -->
          <div v-if="connectionResult.details.connection_info && Object.keys(connectionResult.details.connection_info).length > 0" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Info:</h4>
            <div class="p-3 bg-gray-50 rounded-md text-sm overflow-x-auto">
              <div v-for="(value, key) in connectionResult.details.connection_info" :key="key" class="mb-1 flex">
                <span class="font-medium text-gray-600 mr-2 min-w-[120px]">{{ key }}:</span>
                <span v-if="Array.isArray(value)" class="text-gray-800 break-all">{{ value.join(', ') }}</span>
                <span v-else class="text-gray-800 break-all">{{ value }}</span>
              </div>
            </div>
          </div>
          
          <!-- Connection Errors -->
          <div v-if="connectionResult.details.connection_errors && connectionResult.details.connection_errors.length > 0" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Issues:</h4>
            <ul class="space-y-2">
              <li v-for="(error, index) in connectionResult.details.connection_errors" :key="index" 
                  class="p-3 border rounded-md"
                  :class="{
                    'border-red-200 bg-red-50': error.severity === 'error',
                    'border-yellow-200 bg-yellow-50': error.severity === 'warning',
                    'border-blue-200 bg-blue-50': error.severity === 'info'
                  }">
                <div class="flex items-center mb-1">
                  <span class="w-2 h-2 rounded-full mr-2" 
                        :class="{
                          'bg-red-500': error.severity === 'error',
                          'bg-yellow-500': error.severity === 'warning',
                          'bg-blue-500': error.severity === 'info'
                        }"></span>
                  <span class="text-xs text-gray-500 font-medium">{{ error.severity }}</span>
                </div>
                <p class="text-sm break-words">{{ error.message }}</p>
              </li>
            </ul>
          </div>
          
          <!-- Connection Warnings -->
          <div v-if="connectionResult.details.connection_warnings && connectionResult.details.connection_warnings.length > 0" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Warnings:</h4>
            <ul class="space-y-2">
              <li v-for="(warning, index) in connectionResult.details.connection_warnings" :key="index" 
                  class="p-3 border border-yellow-200 bg-yellow-50 rounded-md">
                <div class="flex items-center mb-1">
                  <span class="w-2 h-2 rounded-full mr-2 bg-yellow-500"></span>
                  <span class="text-xs text-gray-500 font-medium">{{ warning.severity || 'warning' }}</span>
                </div>
                <p class="text-sm break-words">{{ warning.message }}</p>
              </li>
            </ul>
          </div>
          
          <!-- No Connection Info -->
          <div v-if="!connectionResult.details.client_type && 
                    !connectionResult.details.connection_status && 
                    (!connectionResult.details.connection_info || Object.keys(connectionResult.details.connection_info).length === 0)"
               class="text-center py-4 text-gray-500">
            No connection information available
          </div>
        </div>
        
        <div class="flex justify-end mt-4">
          <button @click="closeConnectionModal" class="btn btn-secondary btn-md">Close</button>
        </div>
      </div>
    </div>

    <!-- Test Plugin Modal -->
    <div v-if="showTestPluginModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-[500px] max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Test Plugin: {{ testPluginName }}</h3>
          <button @click="closeTestPluginModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <!-- Plugin Arguments -->
        <div class="mb-6">
          <h3 class="text-lg font-medium text-gray-800 mb-4">Plugin Arguments</h3>
          <div class="space-y-3">
            <div v-for="(arg, index) in testPluginArgs" :key="index" class="flex items-center space-x-3">
              <div class="flex-1">
                <label class="block text-sm font-medium text-gray-700 mb-1">
                  Argument {{ index + 1 }}
                </label>
                <input 
                  v-model="arg.value" 
                  :placeholder="`Enter argument ${index + 1} value...`"
                  class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
                <div class="text-xs text-gray-500 mt-1">
                  String, number, or boolean value
                </div>
              </div>
              <button 
                @click="removePluginArg(index)" 
                class="btn btn-icon btn-danger-ghost"
                :disabled="testPluginArgs.length === 1"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                </svg>
              </button>
            </div>
          </div>
          
          <button @click="addPluginArg" class="btn btn-secondary-ghost btn-sm mt-3">
            <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
            </svg>
            Add Argument
          </button>
        </div>

        <!-- Test Results -->
        <div v-if="testPluginExecuted" class="mb-6">
          <h3 class="text-lg font-medium text-gray-800 mb-4">Test Results</h3>
          
          <div v-if="testPluginLoading" class="flex items-center justify-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
            <span class="ml-3 text-gray-600">Running test...</span>
          </div>
          
          <div v-else-if="testPluginError" class="bg-red-50 border-l-4 border-red-500 p-4 rounded-md">
            <div class="text-red-700 font-medium mb-2">Error</div>
            <pre class="text-red-600 text-sm whitespace-pre-wrap">{{ testPluginError }}</pre>
          </div>
          
          <div v-else class="bg-gray-50 border border-gray-200 rounded-md p-4">
            <div class="mb-3">
              <div class="text-sm font-medium text-gray-700 mb-2">Status:</div>
              <span :class="testPluginResult.success ? 'text-green-600 bg-green-100' : 'text-red-600 bg-red-100'" 
                    class="px-2 py-1 rounded text-sm font-medium">
                {{ testPluginResult.success ? 'Success' : 'Failed' }}
              </span>
            </div>
            
            <div v-if="testPluginResult.result !== null && testPluginResult.result !== undefined">
              <div class="text-sm font-medium text-gray-700 mb-2">Result:</div>
              <div class="bg-white border border-gray-200 rounded overflow-hidden">
                <JsonViewer :value="testPluginResult.result" height="auto" />
              </div>
            </div>
            
            <div v-else class="text-gray-500 italic text-sm">
              No result value returned
            </div>
          </div>
        </div>
        
        <div v-else class="mb-6">
          <div class="text-center py-8 text-gray-400">
            Configure arguments and run test to see results
          </div>
        </div>
        
        <div class="flex justify-end space-x-3">
          <button 
            @click="testPlugin" 
            class="btn btn-test-plugin btn-md"
            :disabled="testPluginLoading"
          >
            <span v-if="testPluginLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
            {{ testPluginLoading ? 'Running...' : 'Run Test' }}
          </button>
          <button @click="closeTestPluginModal" class="btn btn-secondary btn-md">
            Close
          </button>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Modal -->
    <div v-if="showDeleteModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <div class="flex justify-between items-center mb-4">
          <h3 class="text-lg font-medium text-gray-900">Confirm Delete</h3>
          <button @click="closeDeleteModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div class="mb-6">
          <p class="text-sm text-gray-600 mb-2">
            You are about to delete <span class="font-semibold">{{ itemToDelete?.item?.id || itemToDelete?.item?.name }}</span>.
            This action cannot be undone.
          </p>
          <p class="text-sm text-gray-600 mb-4">
            Type <span class="font-bold text-red-600">delete</span> to confirm.
          </p>
          
          <input 
            type="text" 
            v-model="deleteConfirmText" 
            class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-red-500" 
            placeholder="Type 'delete' to confirm"
            @keyup.enter="confirmDelete"
          />
        </div>
        
        <div v-if="deleteError" class="mb-4 text-sm text-red-600">{{ deleteError }}</div>
        
        <div class="flex justify-end space-x-3">
          <button 
            @click="closeDeleteModal" 
            class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors"
          >
            Cancel
          </button>
          <button 
            @click="confirmDelete" 
            class="px-3 py-1.5 bg-red-600 text-white text-sm rounded hover:bg-red-700 transition-colors"
            :disabled="deleteConfirmText !== 'delete'"
          >
            Delete
          </button>
        </div>
      </div>
    </div>

    <!-- Project Warning Modal -->
    <div v-if="showProjectWarningModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <div class="flex items-center mb-4 text-yellow-600">
          <svg class="w-6 h-6 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <h3 class="text-lg font-medium">Warning</h3>
        </div>
        
        <p class="mb-4 text-sm text-gray-600">{{ projectWarningMessage }}</p>
        
        <div class="flex justify-end space-x-3">
          <button @click="closeProjectWarningModal" class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50">
            Cancel
          </button>
          <button @click="continueProjectOperation" class="px-3 py-1.5 bg-yellow-500 text-white text-sm rounded hover:bg-yellow-600" :disabled="projectOperationLoading">
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin mr-1"></span>
            Continue Anyway
          </button>
        </div>
      </div>
    </div>

    <!-- Tooltip component -->
    <div v-if="tooltip.show" 
         class="absolute z-50 bg-gray-800 text-white text-xs rounded py-1 px-2 max-w-xs"
         :style="{
           top: tooltip.y + 'px',
           left: tooltip.x + 'px',
           transform: 'translate(-50%, -100%)',
           marginTop: '-8px'
         }">
      {{ tooltip.text }}
    </div>

    <!-- 添加组件使用情况模态框 -->
    <div v-if="showUsageModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-96 max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Component Usage</h3>
          <button @click="closeUsageModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div v-if="usageLoading" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        
        <div v-else-if="usageError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div class="ml-3">
              <p class="text-sm text-red-700">{{ usageError }}</p>
            </div>
          </div>
        </div>
        
        <div v-else>
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Component Type:</div>
            <div class="font-medium">{{ usageComponentType }}</div>
          </div>
          
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Component ID:</div>
            <div class="font-medium">{{ usageComponentId }}</div>
          </div>
          
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Projects using this component:</div>
            <div v-if="usageProjects.length === 0" class="text-gray-500 italic">
              No projects are using this component
            </div>
            <div v-else class="mt-2 space-y-2">
              <div v-for="project in usageProjects" :key="project.id" 
                   class="p-2 border rounded-md flex items-center justify-between cursor-pointer hover:bg-gray-50 transition-colors"
                   @click="navigateToProject(project.id)">
                <div class="flex items-center">
                  <span class="w-2 h-2 rounded-full mr-2"
                        :class="{
                          'bg-green-500': project.status === 'running',
                          'bg-gray-500': project.status === 'stopped',
                          'bg-blue-500 animate-pulse': project.status === 'starting',
                          'bg-orange-500 animate-pulse': project.status === 'stopping',
                          'bg-red-500': project.status === 'error'
                        }"></span>
                  <span>{{ project.id }}</span>
                </div>
                <div>
                  <span class="text-xs px-2 py-0.5 rounded-full"
                        :class="{
                          'bg-green-100 text-green-800': project.status === 'running',
                          'bg-gray-100 text-gray-800': project.status === 'stopped',
                          'bg-blue-100 text-blue-800 animate-pulse': project.status === 'starting',
                          'bg-orange-100 text-orange-800 animate-pulse': project.status === 'stopping',
                          'bg-red-100 text-red-800': project.status === 'error'
                        }">
                    {{ project.status }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <div class="flex justify-end mt-4">
          <button @click="closeUsageModal" class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded text-sm transition">Close</button>
        </div>
      </div>
    </div>

    <!-- Sample Data Modal -->
    <div v-if="showSampleDataModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-3/4 max-w-6xl">
        <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h3 class="text-lg font-medium">Sample Data - {{ sampleDataComponentType.toUpperCase() }} ({{ sampleDataComponentId }})</h3>
          <button @click="closeSampleDataModal" class="text-gray-400 hover:text-gray-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="p-6 max-h-[70vh] overflow-auto">
          <div v-if="sampleDataLoading" class="flex justify-center items-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </div>
          <div v-else-if="sampleDataError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="ml-3">
                <p class="text-sm text-red-700">{{ sampleDataError }}</p>
              </div>
            </div>
          </div>
          <div v-else-if="!sampleData || Object.keys(sampleData).length === 0" class="text-center text-gray-500 py-8">
            No sample data available
          </div>
          <div v-else class="space-y-6">
            <!-- Grouped by ProjectNodeSequence -->
            <div v-for="(samples, projectNodeSequence) in sampleData" :key="projectNodeSequence" class="border border-gray-200 rounded-lg p-4">
              <div class="mb-3 flex items-center justify-between">
                <h4 class="text-sm font-medium text-gray-700">Project Node Sequence: {{ projectNodeSequence }}</h4>
                <span class="text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-full">{{ samples.length }} samples</span>
              </div>
              <div class="space-y-3">
                <div v-for="(sample, index) in samples.slice(0, 5)" :key="index" class="bg-gray-50 rounded p-3">
                  <div class="text-xs text-gray-500 mb-2 flex justify-between">
                    <span>Sample {{ index + 1 }}</span>
                    <span v-if="sample.timestamp">{{ new Date(sample.timestamp).toLocaleString('en-US', {
                      year: 'numeric',
                      month: '2-digit',
                      day: '2-digit',
                      hour: '2-digit',
                      minute: '2-digit',
                      second: '2-digit',
                      hour12: false
                    }) }}</span>
                  </div>
                  <JsonViewer :value="sample.data || sample" height="auto" />
                </div>
                <div v-if="samples.length > 5" class="text-center">
                  <span class="text-xs text-gray-500">... and {{ samples.length - 5 }} more samples</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    
    <!-- Cluster Status Modal -->
    <div v-if="showClusterStatusModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-2/3 max-w-4xl">
        <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h3 class="text-lg font-medium">Cluster Project Status - {{ selectedProjectForCluster?.name || selectedProjectForCluster?.id }}</h3>
          <button @click="closeClusterStatusModal" class="text-gray-400 hover:text-gray-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="p-6 max-h-[70vh] overflow-auto">
          <div v-if="clusterProjectStatesLoading" class="flex justify-center items-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </div>
          <div v-else-if="clusterProjectStatesError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="ml-3">
                <p class="text-sm text-red-700">{{ clusterProjectStatesError }}</p>
              </div>
            </div>
          </div>
          <div v-else-if="!clusterProjectStates?.project_states || Object.keys(clusterProjectStates.project_states).length === 0" class="text-center text-gray-500 py-8">
            No cluster data available
          </div>
          <div v-else>
            <!-- Simple table showing project status across nodes -->
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                  <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Node</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Role</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Project Status</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Last Updated</th>
                  </tr>
                </thead>
                <tbody class="bg-white divide-y divide-gray-200">
                  <tr v-for="(projects, nodeId) in clusterProjectStates.project_states" :key="nodeId">
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                      {{ nodeId }}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap">
                      <span v-if="nodeId === clusterProjectStates.cluster_status?.node_id" 
                            class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800">
                        Leader
                      </span>
                      <span v-else class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                        Follower
                      </span>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap">
                      <template v-if="projects && getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name)">
                        <span :class="'inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ' + getStatusColorClass(getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name).status)">
                          {{ getStatusDisplayText(getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name).status) }}
                        </span>
                      </template>
                      <template v-else>
                        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-500">
                          {{ projects === null ? 'No Data' : 'Not Found' }}
                        </span>
                      </template>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      <template v-if="projects && getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name)">
                        {{ getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name).status_changed_at ? 
                            new Date(getProjectStatusForNode(projects, selectedProjectForCluster?.id || selectedProjectForCluster?.name).status_changed_at).toLocaleString('en-US', {
                              year: 'numeric',
                              month: '2-digit', 
                              day: '2-digit',
                              hour: '2-digit',
                              minute: '2-digit',
                              second: '2-digit',
                              hour12: false
                            }) : 
                            'Unknown' }}
                      </template>
                      <template v-else>
                        -
                      </template>
                    </td>
                  </tr>
                  <!-- Show empty row if no nodes have project states -->
                  <tr v-if="Object.keys(clusterProjectStates.project_states).length === 0">
                    <td colspan="4" class="px-6 py-4 text-center text-gray-500">
                      No nodes found
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Plugin Stats Modal -->
    <div v-if="showPluginStatsModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-2/3 max-w-2xl">
        <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h3 class="text-lg font-medium">Plugin Statistics - {{ selectedPluginForStats?.id || selectedPluginForStats?.name }}</h3>
          <button @click="closePluginStatsModal" class="text-gray-400 hover:text-gray-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="p-6 max-h-[70vh] overflow-auto">
          <div v-if="pluginStatsLoading" class="flex justify-center items-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </div>
          <div v-else-if="pluginStatsError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="ml-3">
                <p class="text-sm text-red-700">{{ pluginStatsError }}</p>
              </div>
            </div>
          </div>
          <div v-else>
            <!-- Today's Stats -->
            <div class="mb-6">
              <h4 class="text-sm font-medium text-gray-900 mb-3">Today's Statistics</h4>
              <div class="grid grid-cols-2 gap-4">
                <div class="bg-green-50 rounded-lg p-4">
                  <div class="flex items-center">
                    <div class="flex-shrink-0">
                      <svg class="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                    <div class="ml-4">
                      <p class="text-sm font-medium text-green-600">Successful Calls</p>
                      <p class="text-2xl font-bold text-green-900">{{ formatNumber(pluginStatsData.success || 0) }}</p>
                    </div>
                  </div>
                </div>
                <div class="bg-red-50 rounded-lg p-4">
                  <div class="flex items-center">
                    <div class="flex-shrink-0">
                      <svg class="w-8 h-8 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                    <div class="ml-4">
                      <p class="text-sm font-medium text-red-600">Failed Calls</p>
                      <p class="text-2xl font-bold text-red-900">{{ formatNumber(pluginStatsData.failure || 0) }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Success Rate -->
            <div class="mb-6">
              <h4 class="text-sm font-medium text-gray-900 mb-3">Success Rate</h4>
              <div class="bg-gray-50 rounded-lg p-4">
                <div class="flex items-center justify-between">
                  <span class="text-sm font-medium text-gray-600">Success Rate</span>
                  <span class="text-lg font-bold text-gray-900">{{ formatPercent(getPluginSuccessRate()) }}%</span>
                </div>
                <div class="mt-2 w-full bg-gray-200 rounded-full h-2">
                  <div class="bg-green-600 h-2 rounded-full transition-all duration-300" :style="{ width: getPluginSuccessRate() + '%' }"></div>
                </div>
              </div>
            </div>

            <!-- Total Calls -->
            <div>
              <h4 class="text-sm font-medium text-gray-900 mb-3">Total Calls</h4>
              <div class="bg-blue-50 rounded-lg p-4">
                <div class="flex items-center">
                  <div class="flex-shrink-0">
                    <svg class="w-8 h-8 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                    </svg>
                  </div>
                  <div class="ml-4">
                    <p class="text-sm font-medium text-blue-600">Total Invocations</p>
                    <p class="text-2xl font-bold text-blue-900">{{ formatNumber((pluginStatsData.success || 0) + (pluginStatsData.failure || 0)) }}</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Plugin Usage Modal -->
    <div v-if="showPluginUsageModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-3/4 max-w-4xl">
        <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h3 class="text-lg font-medium">Plugin Usage - {{ selectedPluginForUsage?.id || selectedPluginForUsage?.name }}</h3>
          <button @click="closePluginUsageModal" class="text-gray-400 hover:text-gray-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="p-6 max-h-[70vh] overflow-auto">
          <div v-if="pluginUsageLoading" class="flex justify-center items-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </div>
          <div v-else-if="pluginUsageError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="ml-3">
                <p class="text-sm text-red-700">{{ pluginUsageError }}</p>
              </div>
            </div>
          </div>
          <div v-else-if="pluginUsageData">
            <!-- Plugin Usage Content -->
            <div class="space-y-4">
              <!-- Summary -->
              <div class="bg-gray-50 rounded-lg p-4">
                <h4 class="font-medium text-gray-900 mb-2">Usage Summary</h4>
                <div class="grid grid-cols-3 gap-4 text-sm">
                  <div>
                    <span class="text-gray-500">Total Usage:</span>
                    <span class="ml-2 font-semibold">{{ pluginUsageData.total_usage || 0 }}</span>
                  </div>
                  <div>
                    <span class="text-gray-500">Used in Rulesets:</span>
                    <span class="ml-2 font-semibold">{{ pluginUsageData.used_by_rulesets ? pluginUsageData.used_by_rulesets.length : 0 }}</span>
                  </div>
                  <div>
                    <span class="text-gray-500">Used in Projects:</span>
                    <span class="ml-2 font-semibold">{{ pluginUsageData.used_by_projects ? pluginUsageData.used_by_projects.length : 0 }}</span>
                  </div>
                </div>
              </div>
              
              <!-- Rulesets using this plugin -->
              <div v-if="pluginUsageData.used_by_rulesets && pluginUsageData.used_by_rulesets.length > 0">
                <h4 class="font-medium text-gray-900 mb-2">Rulesets Using This Plugin</h4>
                <div class="space-y-2">
                  <div v-for="ruleset in pluginUsageData.used_by_rulesets" :key="ruleset.ruleset_id" 
                       class="bg-white border border-gray-200 rounded-lg p-3">
                    <div class="flex justify-between items-start">
                      <div class="flex-1">
                        <div class="flex items-center mb-1">
                          <span class="font-medium text-gray-900">{{ ruleset.ruleset_id }}</span>
                          <span class="ml-2 text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-full">
                            {{ ruleset.usage_count }} uses
                          </span>
                        </div>
                        <div class="text-sm text-gray-600 mb-1">
                          <span class="font-medium">Usage Types:</span>
                          <span v-for="(type, index) in ruleset.usage_types" :key="type" class="ml-1">
                            <span class="bg-gray-100 text-gray-700 px-2 py-0.5 rounded text-xs">{{ type }}</span>
                            <span v-if="index < ruleset.usage_types.length - 1">, </span>
                          </span>
                        </div>
                        <div v-if="ruleset.rule_ids && ruleset.rule_ids.length > 0" class="text-sm text-gray-600">
                          <span class="font-medium">Rules:</span>
                          <span class="ml-1">{{ ruleset.rule_ids.join(', ') }}</span>
                        </div>
                      </div>
                      <button @click="navigateToRuleset(ruleset.ruleset_id)" 
                              class="ml-2 text-blue-600 hover:text-blue-800 text-sm">
                        View →
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              
              <!-- Projects using this plugin -->
              <div v-if="pluginUsageData.used_by_projects && pluginUsageData.used_by_projects.length > 0">
                <h4 class="font-medium text-gray-900 mb-2">Projects Using This Plugin</h4>
                <div class="space-y-2">
                  <div v-for="project in pluginUsageData.used_by_projects" :key="project.project_id" 
                       class="bg-white border border-gray-200 rounded-lg p-3">
                    <div class="flex justify-between items-start">
                      <div class="flex-1">
                        <div class="flex items-center mb-1">
                          <span class="font-medium text-gray-900">{{ project.project_id }}</span>
                          <span class="ml-2 w-2 h-2 rounded-full" 
                                :class="{
                                  'bg-green-500': project.project_status === 'running',
                                  'bg-gray-500': project.project_status === 'stopped',
                                  'bg-red-500': project.project_status === 'error'
                                }"></span>
                          <span class="ml-1 text-xs text-gray-600">{{ project.project_status }}</span>
                        </div>
                        <div class="text-sm text-gray-600">
                          <span class="font-medium">Through Rulesets:</span>
                          <span class="ml-1">{{ project.ruleset_ids.join(', ') }}</span>
                        </div>
                      </div>
                      <button @click="navigateToProject(project.project_id)" 
                              class="ml-2 text-blue-600 hover:text-blue-800 text-sm">
                        View →
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              
              <!-- No usage message -->
              <div v-if="(!pluginUsageData.used_by_rulesets || pluginUsageData.used_by_rulesets.length === 0) && 
                         (!pluginUsageData.used_by_projects || pluginUsageData.used_by_projects.length === 0)" 
                   class="text-center py-8 text-gray-500">
                <svg class="w-12 h-12 mx-auto mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="text-lg font-medium text-gray-900 mb-2">No Usage Found</p>
                <p class="text-gray-500">This plugin is not currently being used by any rulesets or projects.</p>
              </div>
            </div>
          </div>
          <div v-else class="text-center py-8 text-gray-500">
            No usage data available for this plugin
          </div>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, inject, nextTick, watch, computed } from 'vue'
import { hubApi } from '@/api'
import { useRouter } from 'vue-router'
import JsonViewer from '@/components/JsonViewer.vue'
import { getStatusLabel, getStatusTitle, copyToClipboard, formatNumber, formatPercent } from '../../utils/common'
import { debounce } from '../../utils/performance'
import { useDataCacheStore } from '../../stores/dataCache'
// State management integrated into DataCache
// useListSmartRefresh removed - using unified refresh mechanism in setupProjectStatusRefresh

// Get router instance
const router = useRouter()

// Data cache store
const dataCache = useDataCacheStore()

// Props
const props = defineProps({
  selected: Object,
  collapsed: {
    type: Boolean,
    default: false
  }
})

// Emits
const emit = defineEmits([
  'select-item',
  'open-editor',
  'item-deleted',
  'open-pending-changes',
  'test-ruleset',
  'test-output',
  'test-project',
  'toggle-collapse'
])

// Global message component
const $message = inject('$message', window?.$toast)

// Reactive state
const loading = reactive({
  inputs: false,
  outputs: false,
  rulesets: false,
  plugins: false,
  projects: false
})

const error = reactive({
  inputs: null,
  outputs: null,
  rulesets: null,
  plugins: null,
  projects: null
})

const items = reactive({
  inputs: [],
  outputs: [],
  rulesets: [],
  plugins: [],
  projects: []
})

const collapsed = reactive({
  inputs: true,
  outputs: true,
  rulesets: true,
  plugins: true,
  projects: true,
  settings: true,
  builtinPlugins: true // New state for built-in plugins submenu
})

const sections = reactive({
  inputs: { 
    title: 'Input',
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2"/><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v6m-3-3l3-3 3 3"/></svg>'
  },
  outputs: { 
    title: 'Output',
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2"/><path stroke-linecap="round" stroke-linejoin="round" d="M12 15V9m-3 3l3 3 3-3"/></svg>'
  },
  rulesets: { 
    title: 'Ruleset', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01"/></svg>' 
  },
  plugins: { 
    title: 'Plugin', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M11 4a2 2 0 114 0v1a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-1a2 2 0 100 4h1a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-1a2 2 0 10-4 0v1a1 1 0 01-1 1H7a1 1 0 01-1-1v-3a1 1 0 00-1-1H4a2 2 0 110-4h1a1 1 0 001-1V7a1 1 0 011-1h3a1 1 0 001-1V4z"/></svg>' 
  },
  projects: { 
    title: 'Project', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>' 
  },
  settings: { 
    title: 'Setting', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path></svg>',
    children: [
      { type: 'pending-changes', title: 'Push Changes' },
      { type: 'load-local-components', title: 'Load Local Components' },
      { type: 'cluster', title: 'Cluster', icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12a7 7 0 1114 0 7 7 0 01-14 0zM12 8v4l3 3"></path></svg>' },
      { type: 'operations-history', title: 'Operations History', icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>' },
      { type: 'error-logs', title: 'Error Logs', icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path></svg>' }
    ]
  }
})

const showAddModal = ref(false)
const addType = ref('')
const addName = ref('')
const addRaw = ref('')
const addError = ref('')
// projectRefreshInterval removed - using unified projectStatusRefreshInterval

// Connection check related reactive variables
const showConnectionModal = ref(false)
const connectionResult = ref(null)
const connectionLoading = ref(false)
const connectionError = ref(null)

// Plugin testing related reactive variables
const showTestPluginModal = ref(false)
const testPluginName = ref('')
const testPluginArgs = ref([{ value: '' }])
const testPluginLoading = ref(false)
const testPluginResult = ref(null)
const testPluginError = ref(null)
const testPluginExecuted = ref(false)

// Delete confirmation related reactive variables
const showDeleteModal = ref(false)
const deleteConfirmText = ref('')
const itemToDelete = ref(null)
const deleteError = ref('')

// Project operation states
const projectOperationLoading = ref(false)
const showProjectWarningModal = ref(false)
const projectWarningMessage = ref('')
const projectOperationItem = ref(null)
const lastSidebarOperation = ref(0)
const projectOperationType = ref('') // 'start', 'stop', 'restart'

// Sample Data Modal states
const showSampleDataModal = ref(false)
const sampleDataComponentType = ref('')
const sampleDataComponentId = ref('')
const sampleDataLoading = ref(false)
const sampleDataError = ref(null)
const sampleData = ref({})

// Cluster Status Modal states
const showClusterStatusModal = ref(false)
const selectedProjectForCluster = ref(null)
const clusterProjectStatesLoading = ref(false)
const clusterProjectStatesError = ref(null)
const clusterProjectStates = ref({})

// Cluster consistency checking
const clusterConsistencyData = ref({})
const clusterConsistencyLoading = ref(false)

// Plugin Stats Modal states
const showPluginStatsModal = ref(false)
const selectedPluginForStats = ref(null)
const pluginStatsData = ref({})
const pluginStatsLoading = ref(false)
const pluginStatsError = ref('')

// Flag variable to track if ESC key listener is added
const escKeyListenerAdded = ref(false)

// Search
const search = ref('')
const searchResults = ref([])
const searchLoading = ref(false)

// Component refs
const sidebarRef = ref(null)

// Polling state variables
const isPollingProject = ref(false)
const activeProjectPollers = new Map()

// Refresh intervals constants
const REFRESH_INTERVALS = {
  POLLING_INTERVAL: 500, // 0.5 seconds for polling project status
  NORMAL_INTERVAL: 60000, // 60 seconds for normal refresh
  FAST_INTERVAL: 5000    // 5 seconds for fast refresh
}

// Settings menu badges - computed from dataCache
const settingsBadges = computed(() => dataCache.settingsBadges.data)

// Project status refresh functions
function setupProjectStatusRefresh() {
  // This function is no longer needed with the unified cache system
  // Project status refresh is handled by smart refresh system
}

function clearProjectStatusRefresh() {
  // Clear any active project pollers
  activeProjectPollers.clear()
  isPollingProject.value = false
}

// Debounced search function
const debouncedSearch = debounce(async (query) => {
  if (!query || query.length < 2) {
    searchResults.value = []
    return
  }
  
  try {
    searchLoading.value = true
    const response = await hubApi.searchComponents(query)
    searchResults.value = response.results || []
    
    // Auto-expand sections that have search results
    const hasResults = new Set()
    
    // Add sections that have content search results
    searchResults.value.forEach(result => {
      const componentType = result.component_type + 's' // Convert to plural for section keys
      hasResults.add(componentType)
    })
    
    // Add sections that have name matches
    Object.keys(items).forEach(type => {
      if (items[type] && Array.isArray(items[type])) {
        const hasNameMatch = items[type].some(item => {
          const id = item.id || item.name || ''
          return id.toLowerCase().includes(query.toLowerCase())
        })
        if (hasNameMatch) {
          hasResults.add(type)
        }
      }
    })
    
    // Expand all sections with results
    hasResults.forEach(type => {
      if (collapsed[type] !== undefined) {
        collapsed[type] = false
      }
    })
  } catch (error) {
    console.error('Search failed:', error)
    searchResults.value = []
  } finally {
    searchLoading.value = false
  }
}, 300)

// Centralized modal management
const activeModal = ref(null) // Tracks which modal is currently active

// Tooltip state
const tooltip = reactive({
  show: false,
  text: '',
  x: 0,
  y: 0
})

// Add ESC key listener
function addEscKeyListener() {
  if (!escKeyListenerAdded.value) {
    document.addEventListener('keydown', handleEscKey)
    escKeyListenerAdded.value = true
  }
}

// Remove ESC key listener
function removeEscKeyListener() {
  if (escKeyListenerAdded.value) {
    document.removeEventListener('keydown', handleEscKey)
    escKeyListenerAdded.value = false
  }
}

// Handle ESC key press
function handleEscKey(event) {
  if (event.key === 'Escape' && activeModal.value) {
    closeActiveModal()
  }
}

// Close currently active modal
function closeActiveModal() {
  switch (activeModal.value) {
    case 'delete':
      closeDeleteModal()
      break
    case 'add':
      closeAddModal()
      break
    case 'connection':
      closeConnectionModal()
      break
    case 'testPlugin':
      closeTestPluginModal()
      break
    case 'testRuleset':
      closeTestRulesetModal()
      break
    case 'testOutput':
      closeTestOutputModal()
      break
    case 'testProject':
      closeTestProjectModal()
      break
    case 'projectWarning':
      closeProjectWarningModal()
      break
    case 'usage':
      closeUsageModal()
      break
    case 'sampleData':
      closeSampleDataModal()
      break
    case 'clusterStatus':
      closeClusterStatusModal()
      break
    case 'pluginStats':
      closePluginStatsModal()
      break
    case 'pluginUsage':
      closePluginUsageModal()
      break
  }
  
  activeModal.value = null
}

// Lifecycle hooks
onMounted(async () => {
  await fetchAllItems()
  
  // Start cluster consistency checking
  await loadClusterConsistencyData()
  
  // Initialize settings menu badges
  await dataCache.fetchSettingsBadges()
  
  // Add click event listener to close menus when clicking outside
  document.addEventListener('click', handleOutsideClick)
  
  watch(collapsed, (newCollapsed) => {
    dataCache.saveSidebarState(newCollapsed, search.value)
  }, { deep: true })
  
  watch(search, (newSearch) => {
    dataCache.saveSidebarState(collapsed, newSearch)
  })
  
  const restoredState = dataCache.restoreSidebarState()
  if (restoredState) {
    Object.assign(collapsed, restoredState.collapsed)
    search.value = restoredState.search
  }
  
  // Listen for cache clear events to refresh data immediately
  const handleCacheCleared = (event) => {
    const { reason } = event.detail || {};
    console.log(`[Sidebar] Cache cleared: ${reason}, refreshing sidebar data`);
    
    // Refresh all visible component types
    const componentTypes = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects'];
    componentTypes.forEach(type => {
      if (!collapsed[type]) {
        if (type === 'projects') {
          refreshProjectStatus();
        } else {
          fetchItems(type);
        }
      }
    });
    
    // Also refresh cluster consistency data
    loadClusterConsistencyData();
  };
  
  window.addEventListener('cacheCleared', handleCacheCleared)
  
  // Listen for pending changes and local changes events to update badges
  window.addEventListener('pendingChangesApplied', handlePendingChangesApplied)
  window.addEventListener('localChangesLoaded', handleLocalChangesLoaded)
  
  // Set periodic refresh for settings menu badges (every 6 seconds)
  const settingsBadgeInterval = setInterval(() => dataCache.fetchSettingsBadges(), 6 * 1000)
  window._settingsBadgeInterval = settingsBadgeInterval
  
  // Store event handler for cleanup
  window._sidebarCacheHandler = handleCacheCleared
})

onBeforeUnmount(() => {
  // Clear project status refresh timer (handled by clearProjectStatusRefresh)
  clearProjectStatusRefresh()
  
  // Remove ESC key listener
  removeEscKeyListener()
  
  // Remove click event listener
  document.removeEventListener('click', handleOutsideClick)
  
  // Remove pending changes applied event listener
  window.removeEventListener('pendingChangesApplied', handlePendingChangesApplied)
  
  // Remove local changes loaded event listener
  window.removeEventListener('localChangesLoaded', handleLocalChangesLoaded)
  
  // Remove cache cleared event listener
  if (window._sidebarCacheHandler) {
    window.removeEventListener('cacheCleared', window._sidebarCacheHandler)
    delete window._sidebarCacheHandler
  }
  
  // Clear settings badges refresh timer
  if (window._settingsBadgeInterval) {
    clearInterval(window._settingsBadgeInterval)
    delete window._settingsBadgeInterval
  }
})

// Watch for search input changes
watch(search, (newVal) => {
  if (!newVal || newVal.length < 2) {
    searchResults.value = []
    searchLoading.value = false
  }
})

function openAddModal(type) {
  addType.value = type
  addName.value = ''
  addError.value = ''
  showAddModal.value = true
  activeModal.value = 'add'
  
  addEscKeyListener()
  
  // Auto focus on input field when modal opens
  nextTick(() => {
    const inputElement = document.querySelector('input[ref="addNameInput"]') || 
                        document.querySelector('.bg-white input[type="text"]')
    if (inputElement) {
      inputElement.focus()
    }
  })
}

function closeAddModal() {
  showAddModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

async function toggleCollapse(type) {
  collapsed[type] = !collapsed[type]
  // If expanding, refresh the list
  if (!collapsed[type]) {
    // Only fetch items for real component types, not for sections with children
    const section = sections[type]
    if (section && !section.children) {
      if (type === 'projects') {
        // For projects, use lazy refresh first, if list is empty then do a full refresh
        await refreshProjectStatus()
      } else {
        await fetchItems(type)
      }
    }
  }
}



function filteredItems(type) {
  if (!items[type] || !Array.isArray(items[type])) return []
  if (!search.value) return items[type]
  
  // First, filter by name matches
  const nameMatches = items[type].filter(item => {
    const id = item.id || item.name || ''
    return id.toLowerCase().includes(search.value.toLowerCase())
  })
  
  // Add search match info to name matches
  const nameMatchesWithInfo = nameMatches.map(item => ({
    ...item,
    searchMatch: {
      type: 'name',
      nameMatch: true
    }
  }))
  
  // If we have search results from content search, add them
  if (searchResults.value.length > 0) {
    const contentMatches = searchResults.value
      .filter(result => {
        const componentType = type === 'inputs' ? 'input' : 
                            type === 'outputs' ? 'output' :
                            type === 'rulesets' ? 'ruleset' :
                            type === 'projects' ? 'project' :
                            type === 'plugins' ? 'plugin' : ''
        return result.component_type === componentType
      })
      .map(result => {
        // Find the original item
        const originalItem = items[type].find(item => 
          (item.id || item.name) === result.component_id
        )
        
        if (originalItem) {
          return {
            ...originalItem,
            searchMatch: {
              type: 'content',
              lineNumber: result.line_number,
              lineContent: result.line_content,
              isTemporary: result.is_temporary
            }
          }
        }
        return null
      })
      .filter(Boolean)
    
    // Combine results, avoid duplicates
    const allResults = [...nameMatchesWithInfo]
    
    contentMatches.forEach(contentMatch => {
      const existingIndex = allResults.findIndex(item => 
        (item.id || item.name) === (contentMatch.id || contentMatch.name)
      )
      
      if (existingIndex >= 0) {
        // Item already exists from name match, mark as both
        allResults[existingIndex] = {
          ...allResults[existingIndex],
          searchMatch: {
            type: 'both',
            nameMatch: true,
            lineNumber: contentMatch.searchMatch.lineNumber,
            lineContent: contentMatch.searchMatch.lineContent,
            isTemporary: contentMatch.searchMatch.isTemporary
          }
        }
      } else {
        // New item from content search only
        allResults.push(contentMatch)
      }
    })
    
    return allResults
  }
  
  return nameMatchesWithInfo
}

// Get organized plugin items with built-in plugins in a submenu
function getOrganizedPlugins() {
  const allPlugins = filteredItems('plugins')
  const builtinPlugins = allPlugins.filter(item => item.type === 'local')
  const customPlugins = allPlugins.filter(item => item.type !== 'local')
  
  return {
    builtinPlugins,
    customPlugins
  }
}

// Determine if a plugin should be displayed as Plugin Node (P badge) based on its usage
function isPluginNodeType(item) {
  // Special case: pushMsgTo* plugins are used for plugin nodes, not check nodes
  if (item.id && item.id.startsWith('pushMsgTo')) {
    return true
  }
  // Normal case: check returnType
  return item.returnType === 'interface{}'
}

// Determine if a plugin should be displayed as Check Node (C badge)
function isCheckNodeType(item) {
  // Special case: pushMsgTo* plugins are used for plugin nodes, not check nodes
  if (item.id && item.id.startsWith('pushMsgTo')) {
    return false
  }
  // Normal case: check returnType
  return item.returnType === 'bool'
}

async function fetchAllItems() {
  const types = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects']
  await Promise.all(types.map(type => fetchItems(type)))
}

async function fetchItems(type) {
  loading[type] = true
  error[type] = null
  
  try {
    const response = await dataCache.fetchComponents(type)
    
    if (Array.isArray(response)) {
      items[type] = response.map(item => {
        if (type === 'plugins') {
          // Plugin items use name field
          if (!item.name) {
            console.warn(`Skipping invalid ${type} item:`, item);
            return null;
          }
          return {
            id: item.id,
            name: item.name,
            type: item.type,
            status: item.status,
            hasTemp: item.hasTemp,
            returnType: item.returnType
          }
        } else {
          // Other components must have an id field
          if (!item.id) {
            console.warn(`Skipping invalid ${type} item:`, item);
            return null;
          }
          

          
          return {
            id: item.id,
            type: item.type,
            status: item.status,
            hasTemp: item.hasTemp,
            errorMessage: item.errorMessage || ''
          }
        }
      }).filter(Boolean) // Filter out null items
      
      // Sort list by ID
      items[type].sort((a, b) => {
        const idA = a.id || a.name || ''
        const idB = b.id || b.name || ''
        return idA.localeCompare(idB)
      })
    } else {
      items[type] = []
    }
  } catch (err) {
    error[type] = `Failed to load ${type}: ${err.message}`
  } finally {
    loading[type] = false
  }
}

// Note: refreshProjectStatus moved to end of file with polling conflict optimization

// Complete refresh of project list (for initial load and project additions/deletions)
async function fetchProjectsComplete() {
  const type = 'projects'
  loading[type] = true
  error[type] = null
  
  try {
    const response = await dataCache.fetchComponents(type)
    
    if (Array.isArray(response)) {
      items[type] = response.map(item => {
        if (!item.id) {
          console.warn(`Skipping invalid ${type} item:`, item)
          return null
        }
        

        
        return {
          id: item.id,
          type: item.type,
          status: item.status,
          hasTemp: item.hasTemp,
          errorMessage: item.errorMessage || ''
        }
      }).filter(Boolean)
      
      // Sort list by ID
      items[type].sort((a, b) => {
        const idA = a.id || a.name || ''
        const idB = b.id || b.name || ''
        return idA.localeCompare(idB)
      })
    } else {
      items[type] = []
    }
  } catch (err) {
    error[type] = `Failed to load ${type}: ${err.message}`
  } finally {
    loading[type] = false
  }
}

async function confirmAddName() {
  if (!addName.value || addName.value.trim() === '') {
    addError.value = 'Name cannot be empty'
    return
  }
  
  // Normalize name by removing whitespace
  addName.value = addName.value.trim()

  try {
    const raw = ''
    switch (addType.value) {
      case 'inputs':
        await hubApi.createInput(addName.value, raw)
        break
      case 'outputs':
        await hubApi.createOutput(addName.value, raw)
        break
      case 'rulesets':
        await hubApi.createRuleset(addName.value, raw)
        break
      case 'projects':
        await hubApi.createProject(addName.value, raw)
        break
      case 'plugins':
        await hubApi.createPlugin(addName.value, raw)
        break
      default:
        throw new Error('Unsupported type')
    }
    
    // Trigger global event for immediate cache refresh
    window.dispatchEvent(new CustomEvent('componentChanged', {
      detail: { action: 'created', type: addType.value.slice(0, -1), id: addName.value } // Remove 's' from type
    }))
    
    // Refresh the list - for creation, we need to rebuild the list structure
    if (addType.value === 'projects') {
      await fetchProjectsComplete()
    } else {
      await fetchItems(addType.value)
    }
    
    // Close the modal
    showAddModal.value = false
    
    // Directly open edit mode
    emit('open-editor', { 
      type: addType.value, 
      id: addName.value, 
      isEdit: true 
    })
  } catch (e) {
    addError.value = 'Creation failed: ' + (e?.message || 'Unknown error')
  }
}

async function copyName(item) {
  const text = item.id || item.name
  const success = await copyToClipboard(text)
  if (success) {
    // 可选：显示复制成功提示
    // $message?.success?.('Name copied to clipboard')
  }
  closeAllMenus()
}

// Open delete confirmation modal
function openDeleteModal(type, item) {
  closeAllMenus()
  itemToDelete.value = { type, item }
  deleteConfirmText.value = ''
  deleteError.value = ''
  showDeleteModal.value = true
  activeModal.value = 'delete'
  
  addEscKeyListener()
}

// Close delete confirmation modal
function closeDeleteModal() {
  showDeleteModal.value = false
  itemToDelete.value = null
  deleteConfirmText.value = ''
  deleteError.value = ''
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Confirm delete
async function confirmDelete() {
  if (deleteConfirmText.value !== 'delete') {
    deleteError.value = 'Please type "delete" to confirm'
    return
  }
  
  if (!itemToDelete.value) {
    closeDeleteModal()
    return
  }
  
  const { type, item } = itemToDelete.value
  
  try {
    if (type === 'inputs') await hubApi.deleteInput(item.id)
    else if (type === 'outputs') await hubApi.deleteOutput(item.id)
    else if (type === 'rulesets') await hubApi.deleteRuleset(item.id)
    else if (type === 'projects') await hubApi.deleteProject(item.id)
    else if (type === 'plugins') await hubApi.deletePlugin(item.id)
    
    // Refresh the list - for project deletion, we need to rebuild the list structure
    if (type === 'projects') {
      await fetchProjectsComplete()
    } else {
      await fetchItems(type)
    }
    
    // Show success message
    $message?.success?.('Deleted successfully!')
    
    // Emit delete event to notify parent component
    emit('item-deleted', { type, id: item.id })
    
    // Also trigger global event for immediate cache refresh
    window.dispatchEvent(new CustomEvent('componentChanged', {
      detail: { action: 'deleted', type: type.slice(0, -1), id: item.id } // Remove 's' from type (e.g., 'inputs' -> 'input')
    }))
    
    // If the currently selected item is the one being deleted, clear selection
    if (props.selected && props.selected.type === type && props.selected.id === item.id) {
      emit('select-item', { type: null, id: null })
    }
    
    // Close modal
    closeDeleteModal()
  } catch (e) {
    deleteError.value = 'Delete failed: ' + (e?.message || 'Unknown error')
  }
}

function closeAllMenus() {
  // Close all dropdown menus
  Object.keys(items).forEach(type => {
    if (Array.isArray(items[type])) {
      items[type].forEach(item => {
        if (item.menuOpen) {
          item.menuOpen = false
        }
      })
    }
  })
}

// Implement connection check function
async function checkConnection(type, item) {
  closeAllMenus()
  try {
    connectionLoading.value = true
    connectionError.value = null
    showConnectionModal.value = true
    activeModal.value = 'connection'
    
    addEscKeyListener()
    
    const id = item.id || item.name
    const result = await hubApi.connectCheck(type, id)
    connectionResult.value = result
    
    // Handle error response format
    if (result && result.status === 'error') {
      // Try to get detailed error information
      let errorMessage = result.message || 'Connection check failed';
      
      // Check if detailed connection error information is available
      if (result.details && result.details.connection_errors && result.details.connection_errors.length > 0) {
        const detailError = result.details.connection_errors[0].message;
        if (detailError && detailError !== errorMessage) {
          errorMessage = `${errorMessage}: ${detailError}`;
        }
      }
      
      connectionError.value = errorMessage;
    }
  } catch (error) {
    connectionError.value = error.message || 'Failed to check connection'
  } finally {
    connectionLoading.value = false
  }
}

// Close connection check modal
function closeConnectionModal() {
  showConnectionModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Open test plugin modal
function openTestPlugin(item) {
  closeAllMenus()
  testPluginName.value = item.name || item.id
  testPluginArgs.value = [{ value: '' }]
  testPluginResult.value = null
  testPluginError.value = null
  testPluginExecuted.value = false
  showTestPluginModal.value = true
  activeModal.value = 'testPlugin'
  
  addEscKeyListener()
}

// Close test plugin modal
function closeTestPluginModal() {
  showTestPluginModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Open test ruleset modal
function openTestRuleset(item) {
  const payload = {
    type: 'rulesets', 
    id: item.id || item.name
  };
  emit('test-ruleset', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Open test output modal
function openTestOutput(item) {
  const payload = {
    type: 'outputs', 
    id: item.id || item.name
  };
  emit('test-output', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Open test project modal
function openTestProject(item) {
  const payload = {
    type: 'projects', 
    id: item.id || item.name
  };
  emit('test-project', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Open cluster status modal
function openClusterStatusModal(item) {
      // console.log('Opening cluster status modal for project:', item.id || item.name);
  
  // Ensure all menus are closed first
  closeAllMenus();
  
  // Set the selected project and modal state
  selectedProjectForCluster.value = item;
  showClusterStatusModal.value = true;
  activeModal.value = 'clusterStatus';
  
  // Load cluster project states
  loadClusterProjectStates(item.id || item.name);
  
  // Add ESC key listener
  addEscKeyListener();
  
  // Prevent any potential navigation by stopping event propagation
  // This is handled in the template with @click.prevent.stop but adding extra safety
}

// Open plugin stats modal
function openPluginStatsModal(item) {
      // console.log('Opening plugin stats modal for plugin:', item.id || item.name);
  
  // Ensure all menus are closed first
  closeAllMenus();
  
  // Set the selected plugin and modal state
  selectedPluginForStats.value = item;
  showPluginStatsModal.value = true;
  activeModal.value = 'pluginStats';
  
  // Load plugin statistics
  loadPluginStats(item.id || item.name);
  
  // Add ESC key listener
  addEscKeyListener();
}

// Close plugin stats modal
function closePluginStatsModal() {
  showPluginStatsModal.value = false;
  selectedPluginForStats.value = null;
  pluginStatsData.value = {};
  pluginStatsLoading.value = false;
  pluginStatsError.value = '';
  activeModal.value = null;
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener();
  }
}

// Load plugin statistics
async function loadPluginStats(pluginName) {
  pluginStatsLoading.value = true;
  pluginStatsError.value = '';
  
  try {
    const today = new Date().toISOString().split('T')[0];
    // Updated plugin stats call - explicitly request aggregated data for specific plugin
    const response = await hubApi.getPluginStats({ 
      date: today, 
      plugin: pluginName,
      // No need to specify node_id or by_node - defaults to aggregated across all nodes
    });
    
    if (response && response.stats && response.stats[pluginName]) {
      const stats = response.stats[pluginName];
      // Handle both uppercase and lowercase formats from backend
      pluginStatsData.value = {
        success: stats.success || stats.SUCCESS || 0,
        failure: stats.failure || stats.FAILURE || 0
      };
    } else {
      pluginStatsData.value = { success: 0, failure: 0 };
    }
  } catch (error) {
    console.error('Failed to load plugin stats:', error);
    pluginStatsError.value = 'Failed to load plugin statistics: ' + (error.message || 'Unknown error');
    pluginStatsData.value = { success: 0, failure: 0 };
  } finally {
    pluginStatsLoading.value = false;
  }
}

// Get plugin success rate
function getPluginSuccessRate() {
  const success = pluginStatsData.value.success || 0;
  const failure = pluginStatsData.value.failure || 0;
  const total = success + failure;
  
  if (total === 0) return 0;
  return Math.round((success / total) * 100);
}

// Add plugin parameter
function addPluginArg() {
  testPluginArgs.value.push({ value: '' })
}

// Remove plugin parameter
function removePluginArg(index) {
  testPluginArgs.value.splice(index, 1)
  if (testPluginArgs.value.length === 0) {
    testPluginArgs.value.push({ value: '' })
  }
}

// Test plugin
async function testPlugin() {
  testPluginLoading.value = true
  testPluginError.value = null
  testPluginResult.value = {}
  testPluginExecuted.value = true
  
  try {
    // Process parameter values, try to convert to appropriate types
    const args = testPluginArgs.value.map(arg => {
      const value = arg.value.trim()
      if (value === '') return null
      if (value === 'true') return true
      if (value === 'false') return false
      if (!isNaN(value)) return Number(value)
      return value
    })
    
    const result = await hubApi.testPlugin(testPluginName.value, args)
    testPluginResult.value = result
    
    // Handle error message
    if (result.error) {
      testPluginError.value = result.error
    }
  } catch (error) {
    testPluginError.value = error.message || 'Failed to test plugin'
    testPluginResult.value = { 
      success: false, 
      result: null,
      error: error.message || 'Unknown error occurred'
    }
  } finally {
    testPluginLoading.value = false
  }
}

// Get parameter type hint
function getArgumentTypeHint() {
  // Default hint
  return 'String, number, or boolean value'
}

// Use smart refresh system for automatic updates

// Smart refresh handles timing automatically based on transition states
  
  const refreshSidebar = async () => {
    try {
      // Refresh expanded component types (including project status)
      const componentTypes = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects']
      const promises = []
      
      componentTypes.forEach(type => {
        if (!collapsed[type]) {
          if (type === 'projects') {
            promises.push(refreshProjectStatus())
          } else {
            promises.push(fetchItems(type))
          }
        }
      })
      
      // Execute component refresh and cluster data update in parallel
      await Promise.all([
        ...promises,
        loadClusterConsistencyData(),
        showClusterStatusModal.value ? loadClusterProjectStates() : Promise.resolve()
      ])
      
    } catch (error) {
      console.error('Failed to refresh sidebar:', error)
    }
  }

// Removed manual refresh functions - using smart refresh system

// Expose methods to parent component
defineExpose({
  fetchItems,
  fetchAllItems,
  refreshProjectStatus,
  fetchProjectsComplete,
  // State properties for state manager
  collapsed,
  selected: props.selected,
  search,
  activeModal,
  sidebarRef
})

// Debounce click to prevent double click from triggering single click
let clickTimeout = null;

function handleItemClick(type, item) {
  // Clear any existing timeout
  if (clickTimeout) {
    clearTimeout(clickTimeout);
    clickTimeout = null;
    return; // This was a double click, don't handle single click
  }
  
  // Set timeout to handle single click
  clickTimeout = setTimeout(() => {
    const id = item.id || item.name;
    emit('select-item', { type, id });
    clickTimeout = null;
  }, 200); // 200ms delay to detect double click
}

function handleItemDoubleClick(type, item) {
  // Clear the single click timeout
  if (clickTimeout) {
    clearTimeout(clickTimeout);
    clickTimeout = null;
  }
  
  // Don't allow editing for built-in plugins
  if (type === 'plugins' && item.type === 'local') {
    return;
  }
  
  const id = item.id || item.name;
  // Double click opens editor in edit mode
  emit('open-editor', { type, id, isEdit: true });
}

// Send global project operation event
function emitSidebarProjectOperation(operationType, projectId) {
  const timestamp = Date.now()
  lastSidebarOperation.value = timestamp
  
  // Send global project operation event
  window.dispatchEvent(new CustomEvent('projectOperation', {
    detail: {
      projectId,
      operationType,
      timestamp
    }
  }))
}

// Project operations
async function startProject(item) {
  // Record operation time and notify other components
  emitSidebarProjectOperation('start', item.id)
  
  closeAllMenus()
  
  // Step 1: Immediately set UI to starting state for instant user feedback
  if (item && items.projects) {
    const projectItem = items.projects.find(p => p.id === item.id)
    if (projectItem) {
      projectItem.status = 'starting'
      // console.log(`UI state set to 'starting' for project ${item.id}`)
    }
  }
  
  projectOperationLoading.value = true
  
  try {
    // Step 2: Call API (this may take time, but UI already shows feedback)
    const result = await hubApi.startProject(item.id)
    
    if (result.warning) {
      // If there are warnings (e.g., temporary files exist), show warning modal
      projectWarningMessage.value = result.message
      projectOperationItem.value = item
      projectOperationType.value = 'start'
      showProjectWarningModal.value = true
      activeModal.value = 'projectWarning'
      addEscKeyListener()
      
      // Reset UI state since user needs to confirm
      if (item && items.projects) {
        const projectItem = items.projects.find(p => p.id === item.id)
        if (projectItem) {
          projectItem.status = 'stopped'
        }
      }
    } else {
      // Step 3: API succeeded, start polling for real status
      $message?.success?.('Project start command sent successfully')
      
      // Clear all cache since project start affects multiple data types
      dataCache.clearAll()
      
      pollProjectStatusUntilStable(item.id, 'starting')
    }
  } catch (error) {
    // Step 4: API failed, reset UI state and show error
    console.error('Start project API failed:', error)
    $message?.error?.(`Failed to start project: ${error.message || error}`)
    
    if (item && items.projects) {
      const projectItem = items.projects.find(p => p.id === item.id)
      if (projectItem) {
        projectItem.status = 'stopped' // Reset to original state
      }
    }
  } finally {
    projectOperationLoading.value = false
  }
}

async function stopProject(item) {
  // Record operation time and notify other components
  emitSidebarProjectOperation('stop', item.id)
  
  closeAllMenus()
  
  // Step 1: Immediately set UI to stopping state for instant user feedback
  if (item && items.projects) {
    const projectItem = items.projects.find(p => p.id === item.id)
    if (projectItem) {
      projectItem.status = 'stopping'
      // console.log(`UI state set to 'stopping' for project ${item.id}`)
    }
  }
  
  projectOperationLoading.value = true
  
  try {
    // Step 2: Call API
    const result = await hubApi.stopProject(item.id)
    
    // Step 3: API succeeded, start polling for real status
    $message?.success?.('Project stop command sent successfully')
    
    // Clear all cache since project stop affects multiple data types
    dataCache.clearAll()
    
    pollProjectStatusUntilStable(item.id, 'stopping')
  } catch (error) {
    // Step 4: API failed, reset UI state and show error
    console.error('Stop project API failed:', error)
    $message?.error?.(`Failed to stop project: ${error.message || error}`)
    
    if (item && items.projects) {
      const projectItem = items.projects.find(p => p.id === item.id)
      if (projectItem) {
        projectItem.status = 'running' // Reset to original state
      }
    }
  } finally {
    projectOperationLoading.value = false
  }
}

async function restartProject(item) {
  // Record operation time and notify other components
  emitSidebarProjectOperation('restart', item.id)
  
  closeAllMenus()
  
  // Step 1: Immediately set UI to stopping state (restart starts with stop)
  if (item && items.projects) {
    const projectItem = items.projects.find(p => p.id === item.id)
    if (projectItem) {
      projectItem.status = 'stopping'
      // console.log(`UI state set to 'stopping' for project restart ${item.id}`)
    }
  }
  
  projectOperationLoading.value = true
  
  try {
    // Step 2: Call API
    const result = await hubApi.restartProject(item.id)
    
    if (result.warning) {
      // If there are warnings, show warning modal
      projectWarningMessage.value = result.message
      projectOperationItem.value = item
      projectOperationType.value = 'restart'
      showProjectWarningModal.value = true
      activeModal.value = 'projectWarning'
      addEscKeyListener()
      
      // Reset UI state since user needs to confirm
      if (item && items.projects) {
        const projectItem = items.projects.find(p => p.id === item.id)
        if (projectItem) {
          projectItem.status = 'running' // Reset to original state
        }
      }
    } else {
      // Step 3: API succeeded, start polling for real status
      $message?.success?.('Project restart command sent successfully')
      
      // Clear all cache since project restart affects multiple data types
      setTimeout(() => {
        dataCache.clearAll(`project restart: ${item.id}`)
      }, 2000)
      
      // For restart, we expect: stopping -> starting -> running
      pollProjectStatusUntilStable(item.id, 'stopping')
    }
  } catch (error) {
    // Step 4: API failed, reset UI state and show error
    console.error('Restart project API failed:', error)
    $message?.error?.(`Failed to restart project: ${error.message || error}`)
    
    if (item && items.projects) {
      const projectItem = items.projects.find(p => p.id === item.id)
      if (projectItem) {
        projectItem.status = 'running' // Reset to original state
      }
    }
  } finally {
    projectOperationLoading.value = false
  }
}

// New function: Poll project status until it reaches a stable state
async function pollProjectStatusUntilStable(projectId, expectedTransitionState) {
  // Avoid duplicate polling for this project
  if (activeProjectPollers.has(projectId)) {
    return
  }
  activeProjectPollers.set(projectId, true)
  isPollingProject.value = true

  const maxAttempts = 60 // 2 minutes
  const pollInterval = REFRESH_INTERVALS.POLLING_INTERVAL
  let attempts = 0

  const poll = async () => {
    attempts++

    try {
      // Force refresh to bypass cache each time
      const response = await dataCache.fetchComponents('projects', true)
      if (Array.isArray(response)) {
        const project = response.find(p => p.id === projectId)
        if (project) {
          // Update sidebar immediately
          if (items.projects) {
            const sidebarProject = items.projects.find(p => p.id === projectId)
            if (sidebarProject) {
              sidebarProject.status = project.status
              sidebarProject.hasTemp = project.hasTemp
              sidebarProject.errorMessage = project.errorMessage || ''
            }
          }

          // Stable states we wait for
          const stableStates = ['running', 'stopped', 'error', 'failed']
          if (stableStates.includes(project.status)) {
            activeProjectPollers.delete(projectId)
            isPollingProject.value = activeProjectPollers.size > 0
            return
          }

          // Continue polling if still in transition
          if (attempts < maxAttempts) {
            setTimeout(poll, pollInterval)
          } else {
            console.warn(`Project ${projectId} polling timeout after ${maxAttempts} attempts`)
            activeProjectPollers.delete(projectId)
            isPollingProject.value = activeProjectPollers.size > 0
          }
        } else {
          console.warn(`Project ${projectId} not found in response`)
          activeProjectPollers.delete(projectId)
          isPollingProject.value = activeProjectPollers.size > 0
        }
      } else {
        console.warn('Invalid response format for project list')
        activeProjectPollers.delete(projectId)
        isPollingProject.value = activeProjectPollers.size > 0
      }
    } catch (error) {
      console.error(`Error polling project ${projectId} status:`, error)
      if (attempts < maxAttempts) {
        setTimeout(poll, pollInterval)
      } else {
        activeProjectPollers.delete(projectId)
        isPollingProject.value = activeProjectPollers.size > 0
      }
    }
  }

  // Start polling loop
  poll()
}

// Close project operation warning modal
function closeProjectWarningModal() {
  showProjectWarningModal.value = false
  projectWarningMessage.value = ''
  projectOperationItem.value = null
  projectOperationType.value = ''
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Continue project operation (based on original project, not temporary files)
async function continueProjectOperation() {
  if (!projectOperationItem.value || !projectOperationType.value) {
    closeProjectWarningModal()
    return
  }
  
  const item = projectOperationItem.value
  const operationType = projectOperationType.value
  
  closeProjectWarningModal()
  
  // Step 1: Immediately set UI to transition state for instant user feedback
  if (item && items.projects) {
    const projectItem = items.projects.find(p => p.id === item.id)
    if (projectItem) {
      if (operationType === 'start') {
        projectItem.status = 'starting'
        // console.log(`UI state set to 'starting' for project ${item.id} (continue operation)`)
      } else if (operationType === 'stop') {
        projectItem.status = 'stopping'
                  // console.log(`UI state set to 'stopping' for project ${item.id} (continue operation)`)
      } else if (operationType === 'restart') {
        projectItem.status = 'stopping' // Restart starts with stop
                  // console.log(`UI state set to 'stopping' for project restart ${item.id} (continue operation)`)
      }
    }
  }
  
  projectOperationLoading.value = true
  
  try {
    let result
    
    // Step 2: Perform operations on the original project
    if (operationType === 'start') {
      // Start using original project ID
      const response = await hubApi.startProject(item.id)
      result = { success: true, data: response.data }
    } else if (operationType === 'stop') {
      // Stop using original project ID
      const response = await hubApi.stopProject(item.id)
      result = { success: true, data: response.data }
    } else if (operationType === 'restart') {
      // Restart using original project ID
      const response = await hubApi.restartProject(item.id)
      result = { success: true, data: response.data }
    }
    
    if (result && result.success) {
      // Step 3: API succeeded, start polling for real status
      $message?.success?.(`Project ${operationType} command sent successfully`)
      pollProjectStatusUntilStable(item.id, operationType === 'start' ? 'starting' : 'stopping')
    }
  } catch (error) {
    // Step 4: API failed, reset UI state and show error
    console.error(`${operationType} project API failed:`, error)
    $message?.error?.(`Failed to ${operationType} project: ${error.message || error}`)
    
    // Reset UI state based on operation type
    if (item && items.projects) {
      const projectItem = items.projects.find(p => p.id === item.id)
      if (projectItem) {
        if (operationType === 'start') {
          projectItem.status = 'stopped' // Reset to original state
        } else if (operationType === 'stop' || operationType === 'restart') {
          projectItem.status = 'running' // Reset to original state
        }
      }
    }
  } finally {
    projectOperationLoading.value = false
  }
}

// Check if any modal is open
function isAnyModalOpen() {
  // Simply check if there's an active modal
  return activeModal.value !== null;
}

// Handle clicks outside the menu
function handleOutsideClick(event) {
  // Check if the click is inside a dropdown menu or on a menu toggle button
  const isMenuClick = event.target.closest('.dropdown-menu')
  const isToggleClick = event.target.closest('.menu-toggle-button')
  
  // If clicking inside menu or on toggle button, don't close
  if (isMenuClick || isToggleClick) {
    return
  }
  
  // Close all menus
  closeAllMenus()
}

// Show tooltip
function showTooltip(event, text) {
  tooltip.text = text
  tooltip.x = event.clientX
  tooltip.y = event.clientY
  tooltip.show = true
}

// Hide tooltip
function hideTooltip() {
  tooltip.show = false
}

// Variables related to component usage
const showUsageModal = ref(false)
const usageLoading = ref(false)
const usageError = ref(null)
const usageComponentType = ref('')
const usageComponentId = ref('')
const usageProjects = ref([])

// Variables related to plugin usage
const showPluginUsageModal = ref(false)
const pluginUsageLoading = ref(false)
const pluginUsageError = ref(null)
const pluginUsageData = ref(null)
const selectedPluginForUsage = ref(null)

// Add the "View Usage" option to the three-point menu
function openUsageModal(type, item) {
  closeAllMenus()
  usageLoading.value = true
  usageError.value = null
  usageComponentType.value = type
  usageComponentId.value = item.id || item.name
  usageProjects.value = []
  showUsageModal.value = true
  activeModal.value = 'usage'
  
  addEscKeyListener()
  
  // Obtain component usage status
  fetchComponentUsage(type, item.id || item.name)
}

// Open plugin usage modal
function openPluginUsageModal(item) {
  closeAllMenus()
  pluginUsageLoading.value = true
  pluginUsageError.value = null
  pluginUsageData.value = null
  selectedPluginForUsage.value = item
  showPluginUsageModal.value = true
  activeModal.value = 'pluginUsage'
  
  addEscKeyListener()
  
  // Fetch plugin usage data
  fetchPluginUsage(item.id || item.name)
}

// Close the usage mode box
function closeUsageModal() {
  showUsageModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Obtain component usage status
async function fetchComponentUsage(type, id) {
  try {
    const result = await hubApi.getComponentUsage(type, id)
    usageProjects.value = result.usage || []
  } catch (error) {
    usageError.value = error.message || 'Failed to fetch component usage'
  } finally {
    usageLoading.value = false
  }
}

// Fetch plugin usage data
async function fetchPluginUsage(pluginId) {
  try {
    const result = await hubApi.getPluginUsage(pluginId)
    pluginUsageData.value = result.usage || {}
  } catch (error) {
    pluginUsageError.value = error.message || 'Failed to fetch plugin usage'
  } finally {
    pluginUsageLoading.value = false
  }
}

// Close plugin usage modal
function closePluginUsageModal() {
  showPluginUsageModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Jump to the project details page
function navigateToProject(projectId) {
  closeUsageModal()
  closePluginUsageModal()
  
  router.push(`/app/projects/${projectId}`)
  
  // Notify the parent component that a project has been selected
  emit('select-item', {
    type: 'projects',
    id: projectId,
    isEdit: false,
    _timestamp: Date.now()
  })
}

// Navigate to ruleset details page
function navigateToRuleset(rulesetId) {
  closePluginUsageModal()
  
  router.push(`/app/rulesets/${rulesetId}`)
  
  // Notify the parent component that a ruleset has been selected
  emit('select-item', {
    type: 'rulesets',
    id: rulesetId,
    isEdit: false,
    _timestamp: Date.now()
  })
}

// Toggle menu for a specific item
function toggleMenu(item) {
  const wasOpen = item.menuOpen
  
  // Close all menus first
  closeAllMenus()
  
  // If the menu wasn't open, open it
  if (!wasOpen) {
    item.menuOpen = true
  }
}

// Open sample data modal
function openSampleDataModal(item) {
  closeAllMenus()
  
  // Detect component type based on context
  let componentType = 'input' // default
  
  // Find the component type by checking which section this item belongs to
  for (const [type, itemList] of Object.entries(items)) {
    if (itemList.some(i => (i.id || i.name) === (item.id || item.name))) {
      componentType = type.slice(0, -1) // Remove the 's' at the end (inputs -> input)
      break
    }
  }
  
  sampleDataComponentType.value = componentType
  sampleDataComponentId.value = item.id || item.name
  sampleDataLoading.value = true
  sampleDataError.value = null
  sampleData.value = {}
  showSampleDataModal.value = true
  activeModal.value = 'sampleData'
  
  addEscKeyListener()
  
  // Fetch sample data
  fetchSampleData(componentType, item.id || item.name)
}

// Close sample data modal
function closeSampleDataModal() {
  showSampleDataModal.value = false
  sampleDataComponentType.value = ''
  sampleDataComponentId.value = ''
  sampleDataLoading.value = false
  sampleDataError.value = null
  sampleData.value = {}
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Fetch sample data
async function fetchSampleData(componentType, id) {
  try {
    // Get all sample data for this component type and ID across all projects
    // Ensure we don't duplicate the component type prefix
    const projectNodeSequence = id.startsWith(`${componentType}.`) ? id : `${componentType}.${id}`;
    const response = await hubApi.getSamplerData(componentType, projectNodeSequence)
    
    if (response && response[componentType]) {
      // Filter the sample data to only show sequences that belong to this component
      // For a component, we should only show sequences that END with this component,
      // not sequences that pass through this component and continue to other components
      const filteredData = {}
      
      Object.keys(response[componentType]).forEach(projectNodeSequence => {
        // ProjectNodeSequence format: "INPUT.api_sec.RULESET.test" or "RULESET.test"
        // Split the sequence directly by '.'
        const sequenceComponents = projectNodeSequence.split('.')
        
        // Check if this sequence contains our target component
        // Sequence format: component1.id1.component2.id2.component3.id3...
        // We need to check if the sequence contains our component type and id
        if (sequenceComponents.length >= 2 && sequenceComponents.length % 2 === 0) {
          // Look for our component type and id in the sequence
          for (let i = 0; i < sequenceComponents.length - 1; i += 2) {
            const currentComponentType = sequenceComponents[i].toLowerCase()
            const currentComponentId = sequenceComponents[i + 1]
            
            // Check if this component matches our target
            if (currentComponentType === componentType && currentComponentId === id) {
              filteredData[projectNodeSequence] = response[componentType][projectNodeSequence]
              break
            }
          }
        }
      })
      
      sampleData.value = filteredData
    } else {
      sampleData.value = {}
    }
  } catch (error) {
    sampleDataError.value = error.message || 'Failed to fetch sample data'
  } finally {
    sampleDataLoading.value = false
  }
}

function shouldShowConnectCheck(type, item) {
  // For inputs, always show Connect Check
  if (type === 'inputs') {
    return true;
  }
  
  // For outputs, check if it's NOT a print type
  if (type === 'outputs') {
    // Use the type information returned by backend API
    if (item && item.type) {
      const outputType = item.type.toLowerCase();
      if (outputType === 'print') {
        return false; // Print type does NOT show Connect Check
      }
      // For other types (kafka, elasticsearch, aliyun_sls), show Connect Check
      return true;
    }
    
    // Fallback: if no type info, don't show Connect Check (safer)
    return false;
  }
  
  // Other types do NOT show Connect Check
  return false;
}

// Load cluster project states
async function loadClusterProjectStates(projectId) {
  clusterProjectStatesLoading.value = true;
  clusterProjectStatesError.value = null;
  clusterProjectStates.value = {};
  
  try {
    const response = await hubApi.getClusterProjectStates();
    clusterProjectStates.value = response || {};
  } catch (error) {
    clusterProjectStatesError.value = error.message || 'Failed to fetch cluster project states';
  } finally {
    clusterProjectStatesLoading.value = false;
  }
}

// Close cluster status modal
function closeClusterStatusModal() {
  showClusterStatusModal.value = false;
  selectedProjectForCluster.value = null;
  clusterProjectStatesLoading.value = false;
  clusterProjectStatesError.value = null;
  clusterProjectStates.value = {};
  activeModal.value = null;
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener();
  }
}

// Get status display text
function getStatusDisplayText(status) {
  const statusMap = {
    'running': 'Running',
    'stopped': 'Stopped', 
    'starting': 'Starting',
    'stopping': 'Stopping',
    'error': 'Error'
  };
  return statusMap[status] || status;
}

// Get status color class
function getStatusColorClass(status) {
  const colorMap = {
    'running': 'text-green-600 bg-green-100',
    'stopped': 'text-gray-600 bg-gray-100',
    'starting': 'text-blue-600 bg-blue-100', 
    'stopping': 'text-orange-600 bg-orange-100',
    'error': 'text-red-600 bg-red-100'
  };
  return colorMap[status] || 'text-gray-600 bg-gray-100';
}

function getProjectStatusForNode(projects, projectId) {
  if (!projects || !Array.isArray(projects)) {
    return null;
  }
  return projects.find(project => project.id === projectId);
}

// Check if project has cluster status inconsistency
function hasClusterInconsistency(projectId) {
  if (!clusterConsistencyData.value || !clusterConsistencyData.value.project_states) {
    return false;
  }

  const projectStates = clusterConsistencyData.value.project_states;
  const nodeIds = Object.keys(projectStates);
  
  if (nodeIds.length < 2) {
    return false; // Need at least 2 nodes to have inconsistency
  }

  // Collect all statuses from all nodes, treating "No Data" as "stopped"
  let allStatuses = new Set();
  
  for (const nodeId of nodeIds) {
    const projects = projectStates[nodeId];
    if (projects && Array.isArray(projects)) {
      const project = projects.find(p => p.id === projectId);
      allStatuses.add(project ? project.status : 'stopped'); // missing project = stopped
    } else {
      // Node has no project data - treat as "stopped"
      allStatuses.add('stopped');
    }
  }

  // If there's more than one unique status, it's inconsistent
  return allStatuses.size > 1;
}

// Load cluster consistency data in background
async function loadClusterConsistencyData() {
  if (clusterConsistencyLoading.value) {
    return; // Already loading
  }

  clusterConsistencyLoading.value = true;
  try {
    const response = await hubApi.getClusterProjectStates();
    clusterConsistencyData.value = response || {};
  } catch (error) {
    console.warn('Failed to fetch cluster consistency data:', error);
    clusterConsistencyData.value = {};
  } finally {
    clusterConsistencyLoading.value = false;
  }
}

// Handle pending changes applied event
async function handlePendingChangesApplied(event) {
  const { types, timestamp } = event.detail || {}
  
  if (!types || !Array.isArray(types)) {
    return
  }
  
  debouncedFullRefresh()
}

// Handle local changes loaded event
async function handleLocalChangesLoaded(event) {
  const { types, timestamp } = event.detail || {}
  
  if (!types || !Array.isArray(types)) {
    return
  }
  
  debouncedFullRefresh()
}

// Debounced full refresh function
const debouncedFullRefresh = debounce(async () => {
  try {
    // Refresh all component lists
    await fetchAllItems()
    
    // Refresh project status and cluster data
    await refreshProjectStatus()
    await loadClusterConsistencyData()
    if (showClusterStatusModal.value) {
      await loadClusterProjectStates()
    }
    
    // Refresh settings menu badges
    await dataCache.fetchSettingsBadges()
  } catch (error) {
    console.error('Failed to refresh after changes:', error)
  }
}, 500)


async function refreshProjectStatus() {
  // Skip refresh if polling is in progress
  if (isPollingProject.value) {
    return
  }
  
  // If project list is empty or loading, perform complete refresh
  if (!items.projects || items.projects.length === 0 || loading.projects) {
    return await fetchProjectsComplete()
  }
  
  try {
    // Get latest project data using cache
    const response = await dataCache.fetchComponents('projects')
    
    if (Array.isArray(response)) {
      const newProjects = response.map(item => {
        if (!item.id) return null
        return {
          id: item.id,
          type: item.type,
          status: item.status,
          hasTemp: item.hasTemp,
          errorMessage: item.errorMessage || ''
        }
      }).filter(Boolean)
      
      // Check if there are project additions or deletions
      const currentIds = new Set(items.projects.map(p => p.id))
      const newIds = new Set(newProjects.map(p => p.id))
      
      // If project count changed or there are new/deleted projects, perform complete refresh
      if (currentIds.size !== newIds.size || 
          !Array.from(currentIds).every(id => newIds.has(id)) ||
          !Array.from(newIds).every(id => currentIds.has(id))) {
        return await fetchProjectsComplete()
      }
      
      // Only update status of existing projects, don't rebuild list
      items.projects.forEach(currentProject => {
        const updatedProject = newProjects.find(p => p.id === currentProject.id)
        if (updatedProject) {
          // Only update status-related fields to keep DOM stable
          currentProject.status = updatedProject.status
          currentProject.hasTemp = updatedProject.hasTemp
          currentProject.errorMessage = updatedProject.errorMessage
        }
      })
      

    }
  } catch (err) {
    console.error('Failed to refresh project status:', err)
    // If status refresh fails, don't show error, handle silently
  }
}

// Get input type icon and color based on input type
function getInputTypeInfo(item) {
  const type = item.type?.toLowerCase() || 'unknown'
  const typeMap = {
    'kafka': { icon: 'K', color: 'bg-orange-100 text-orange-800', tooltip: 'Kafka Input' },
    'kafka_azure': { icon: 'AK', color: 'bg-blue-100 text-blue-800', tooltip: 'Azure Kafka Input' },
    'kafka_aws': { icon: 'WK', color: 'bg-yellow-100 text-yellow-800', tooltip: 'AWS Kafka Input' },
    'aliyun_sls': { icon: 'SLS', color: 'bg-green-100 text-green-800', tooltip: 'Aliyun SLS Input' },
    'unknown': { icon: '?', color: 'bg-gray-100 text-gray-800', tooltip: 'Unknown Input Type' }
  }
  return typeMap[type] || typeMap['unknown']
}

// Get output type icon and color based on output type
function getOutputTypeInfo(item) {
  const type = item.type?.toLowerCase() || 'unknown'
  const typeMap = {
    'kafka': { icon: 'K', color: 'bg-orange-100 text-orange-800', tooltip: 'Kafka Output' },
    'kafka_azure': { icon: 'AK', color: 'bg-blue-100 text-blue-800', tooltip: 'Azure Kafka Output' },
    'kafka_aws': { icon: 'WK', color: 'bg-yellow-100 text-yellow-800', tooltip: 'AWS Kafka Output' },
    'elasticsearch': { icon: 'ES', color: 'bg-purple-100 text-purple-800', tooltip: 'Elasticsearch Output' },
    'aliyun_sls': { icon: 'SLS', color: 'bg-green-100 text-green-800', tooltip: 'Aliyun SLS Output' },
    'print': { icon: 'P', color: 'bg-gray-100 text-gray-800', tooltip: 'Print Output' },
    'unknown': { icon: '?', color: 'bg-gray-100 text-gray-800', tooltip: 'Unknown Output Type' }
  }
  return typeMap[type] || typeMap['unknown']
}

// Get ruleset type icon and color based on ruleset type
function getRulesetTypeInfo(item) {
  const type = item.type?.toLowerCase() || 'unknown'
  const typeMap = {
    'detection': { icon: 'D', color: 'bg-purple-100 text-purple-800', tooltip: 'Detection Ruleset' },
    'exclude': { icon: 'F', color: 'bg-orange-100 text-orange-800', tooltip: 'Exclude Ruleset' },
    'unknown': { icon: '?', color: 'bg-gray-100 text-gray-800', tooltip: 'Unknown Ruleset Type' }
  }
  return typeMap[type] || typeMap['unknown']
}

</script>

<style>
/* Custom scrollbar for webkit browsers */
.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: #d1d5db;
  border-radius: 3px;
  transition: background-color 0.2s ease;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: #9ca3af;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

/* Firefox scrollbar */
.custom-scrollbar {
  scrollbar-width: thin;
  scrollbar-color: #d1d5db transparent;
}

/* Breathing light effect for starting/stopping states */
.animate-pulse {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: .5;
  }
}
</style> 