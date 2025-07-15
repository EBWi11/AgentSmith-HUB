// 状态管理工具 - 用于保存和恢复UI状态，避免缓存清理后UI重置

class StateManager {
  constructor() {
    this.savedStates = new Map()
  }

  // 保存组件状态
  saveState(componentKey, state) {
    this.savedStates.set(componentKey, {
      ...state,
      timestamp: Date.now()
    })
    console.log(`[StateManager] Saved state for ${componentKey}:`, state)
  }

  // 恢复组件状态
  restoreState(componentKey, maxAge = 30000) { // 30秒内的状态有效
    const savedState = this.savedStates.get(componentKey)
    if (!savedState) {
      return null
    }

    const age = Date.now() - savedState.timestamp
    if (age > maxAge) {
      this.savedStates.delete(componentKey)
      console.log(`[StateManager] State for ${componentKey} expired (${age}ms)`)
      return null
    }

    console.log(`[StateManager] Restored state for ${componentKey}:`, savedState)
    return savedState
  }

  // 清理过期状态
  cleanup(maxAge = 30000) {
    const now = Date.now()
    for (const [key, state] of this.savedStates.entries()) {
      if (now - state.timestamp > maxAge) {
        this.savedStates.delete(key)
      }
    }
  }

  // 清理所有状态
  clear() {
    this.savedStates.clear()
  }
}

// 全局状态管理器实例
export const globalStateManager = new StateManager()

// Sidebar 状态保存和恢复
export function saveSidebarState(sidebarExposed) {
  if (!sidebarExposed) return

  // 获取滚动位置
  const scrollElement = sidebarExposed.sidebarRef?.querySelector('.overflow-y-auto')
  const scrollTop = scrollElement ? scrollElement.scrollTop : 0

  const state = {
    // 展开状态
    collapsed: { ...sidebarExposed.collapsed },
    // 选中状态
    selectedId: sidebarExposed.selected?.id || null,
    selectedType: sidebarExposed.selected?.type || null,
    // 搜索状态
    searchValue: sidebarExposed.search || '',
    // 滚动位置
    scrollTop: scrollTop,
    // 菜单状态
    activeModal: sidebarExposed.activeModal || null
  }

  globalStateManager.saveState('sidebar', state)
  return state
}

export function restoreSidebarState(sidebarExposed) {
  if (!sidebarExposed) return false

  const savedState = globalStateManager.restoreState('sidebar')
  if (!savedState) return false

  try {
    // 恢复展开状态
    if (savedState.collapsed) {
      Object.assign(sidebarExposed.collapsed, savedState.collapsed)
    }

    // 恢复搜索状态
    if (savedState.searchValue !== undefined) {
      sidebarExposed.search = savedState.searchValue
    }

    // 恢复滚动位置
    if (savedState.scrollTop > 0) {
      setTimeout(() => {
        const scrollElement = sidebarExposed.sidebarRef?.querySelector('.overflow-y-auto')
        if (scrollElement) {
          scrollElement.scrollTop = savedState.scrollTop
        }
      }, 100) // 延迟等待DOM更新
    }

    // 恢复选中状态需要在父组件中处理，因为涉及路由
    
    console.log('[StateManager] Sidebar state restored successfully')
    return {
      selectedId: savedState.selectedId,
      selectedType: savedState.selectedType
    }
  } catch (error) {
    console.error('[StateManager] Failed to restore sidebar state:', error)
    return false
  }
}

// ComponentDetail 状态保存和恢复
export function saveComponentDetailState(componentDetailRef) {
  if (!componentDetailRef) return

  const state = {
    // 编辑器状态
    editorValue: componentDetailRef.editorValue || '',
    isEditMode: componentDetailRef.isEditMode || false,
    // 验证状态
    validationResult: componentDetailRef.validationResult || null,
    showValidationPanel: componentDetailRef.showValidationPanel || false,
    // 模态框状态
    activeModal: componentDetailRef.activeModal || null,
    // 当前选中项
    currentItem: componentDetailRef.props?.item ? {
      id: componentDetailRef.props.item.id,
      type: componentDetailRef.props.item.type
    } : null
  }

  globalStateManager.saveState('componentDetail', state)
  return state
}

export function restoreComponentDetailState(componentDetailRef) {
  if (!componentDetailRef) return false

  const savedState = globalStateManager.restoreState('componentDetail')
  if (!savedState) return false

  try {
    // 恢复编辑器状态
    if (savedState.editorValue !== undefined) {
      componentDetailRef.editorValue = savedState.editorValue
    }

    if (savedState.isEditMode !== undefined) {
      componentDetailRef.isEditMode = savedState.isEditMode
    }

    // 恢复验证状态
    if (savedState.validationResult) {
      componentDetailRef.validationResult = savedState.validationResult
    }

    if (savedState.showValidationPanel !== undefined) {
      componentDetailRef.showValidationPanel = savedState.showValidationPanel
    }

    console.log('[StateManager] ComponentDetail state restored successfully')
    return savedState.currentItem
  } catch (error) {
    console.error('[StateManager] Failed to restore componentDetail state:', error)
    return false
  }
}

// ProjectWorkflow 状态保存和恢复
export function saveProjectWorkflowState(workflowRef) {
  if (!workflowRef) return

  const state = {
    // 消息数据
    messageData: workflowRef.messageData?.value || {},
    componentSequences: workflowRef.componentSequences?.value || {},
    // 节点状态
    selectedNode: workflowRef.selectedNode?.value || null,
    // 模态框状态
    showSampleModal: workflowRef.showSampleModal?.value || false,
    showContextMenu: workflowRef.showContextMenu?.value || false
  }

  globalStateManager.saveState('projectWorkflow', state)
  return state
}

export function restoreProjectWorkflowState(workflowRef) {
  if (!workflowRef) return false

  const savedState = globalStateManager.restoreState('projectWorkflow')
  if (!savedState) return false

  try {
    // 恢复消息数据
    if (savedState.messageData && workflowRef.messageData) {
      workflowRef.messageData.value = savedState.messageData
    }

    if (savedState.componentSequences && workflowRef.componentSequences) {
      workflowRef.componentSequences.value = savedState.componentSequences
    }

    // 恢复选中节点
    if (savedState.selectedNode && workflowRef.selectedNode) {
      workflowRef.selectedNode.value = savedState.selectedNode
    }

    console.log('[StateManager] ProjectWorkflow state restored successfully')
    return true
  } catch (error) {
    console.error('[StateManager] Failed to restore projectWorkflow state:', error)
    return false
  }
}

// 定期清理过期状态
setInterval(() => {
  globalStateManager.cleanup()
}, 60000) // 每分钟清理一次 