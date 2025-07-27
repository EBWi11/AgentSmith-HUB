import { ref, computed, onMounted, onUnmounted, watch, readonly } from 'vue'
import { throttle } from '../utils/performance'

export function useSmartRefresh(refreshFunction, options = {}) {
  let {
    baseInterval: baseIntervalMs = 30000,      // Base refresh interval (30s)
    fastInterval: fastIntervalMs = 2000,       // Fast refresh interval (2s)
    slowInterval = 300000,     // Slow refresh interval (5min)
    maxErrorCount = 3,         // Maximum error count
    errorRecoveryInterval = 300000, // Error recovery interval (5min)
    enableUserActivity = true, // Enable user activity detection
    enableNetworkDetection = true, // Enable network status detection
    enableVisibilityDetection = true, // Enable page visibility detection
    transitionStates = [],     // States that need fast refresh
    debug = false              // Debug mode
  } = options

  // State management
  const isRefreshing = ref(false)
  const errorCount = ref(0)
  const lastRefreshTime = ref(0)
  const lastUserActivity = ref(Date.now())
  const isUserActive = ref(true)
  const isNetworkOnline = ref(navigator.onLine)
  const isPageVisible = ref(!document.hidden)
  const currentInterval = ref(baseIntervalMs)
  const refreshTimer = ref(null)
  const hasTransitionStates = ref(false)

  // Log function
  const log = (message, ...args) => {
    if (debug) {
      // console.log(`[SmartRefresh] ${message}`, ...args)
    }
  }

  // Error handling
  const handleError = (error) => {
    errorCount.value++
    log(`Error occurred (${errorCount.value}/${maxErrorCount}):`, error)
    
    if (errorCount.value >= maxErrorCount) {
      log('Max error count reached, entering recovery mode')
      setRefreshInterval(errorRecoveryInterval)
    }
  }

  // Reset error count
  const resetErrorCount = () => {
    if (errorCount.value > 0) {
      errorCount.value = 0
      log('Error count reset')
    }
  }

  // User activity detection
  const updateUserActivity = throttle(() => {
    lastUserActivity.value = Date.now()
    if (!isUserActive.value) {
      isUserActive.value = true
      log('User became active')
    }
  }, 1000)

  // Check user inactivity
  const checkUserInactivity = () => {
    const now = Date.now()
    if (now - lastUserActivity.value > 60000 && isUserActive.value) {
      isUserActive.value = false
      log('User became inactive')
    }
  }

  // Network status listener
  const handleNetworkChange = (online) => {
    isNetworkOnline.value = online
    log(`Network status changed: ${online ? 'online' : 'offline'}`)
    
    if (online) {
      // Refresh immediately when network recovers
      performRefresh()
    }
  }

  // Page visibility listener
  const handleVisibilityChange = () => {
    isPageVisible.value = !document.hidden
    log(`Page visibility changed: ${isPageVisible.value ? 'visible' : 'hidden'}`)
    
    if (isPageVisible.value) {
      // Refresh immediately when page becomes visible
      performRefresh()
    }
  }

  // Calculate optimal refresh interval
  const optimalInterval = computed(() => {
    // If page not visible, use slow refresh
    if (!isPageVisible.value) {
      return slowInterval
    }

    // If network offline, pause refresh
    if (!isNetworkOnline.value) {
      return null
    }

    // If has transition states, use fast refresh
    if (hasTransitionStates.value) {
      return fastIntervalMs
    }

    // If user inactive, use slow refresh
    if (!isUserActive.value) {
      return slowInterval
    }

    // If has errors, use error recovery interval
    if (errorCount.value >= maxErrorCount) {
      return errorRecoveryInterval
    }

    // Default to base interval
    return baseIntervalMs
  })

  // Set refresh interval
  const setRefreshInterval = (interval) => {
    if (refreshTimer.value) {
      clearInterval(refreshTimer.value)
      refreshTimer.value = null
    }

    if (interval && interval > 0) {
      currentInterval.value = interval
      refreshTimer.value = setInterval(performRefresh, interval)
      log(`Refresh interval set to ${interval}ms`)
    } else {
      log('Refresh paused')
    }
  }

  // Perform refresh
  const performRefresh = async () => {
    if (isRefreshing.value) {
      log('Already refreshing, skipping')
      return
    }

    if (!isNetworkOnline.value) {
      log('Network offline, skipping refresh')
      return
    }

    isRefreshing.value = true
    const startTime = performance.now()
    lastRefreshTime.value = Date.now()
    
    try {
      log('Starting refresh')
      await refreshFunction()
      log('Refresh completed successfully')
      resetErrorCount()
    } catch (error) {
      log('Refresh failed:', error)
      handleError(error)
    } finally {
      isRefreshing.value = false
      // Dynamically extend interval if actual refresh duration exceeds current interval
      const duration = performance.now() - startTime
      const minInterval = Math.ceil(duration * 1.2)
      if (currentInterval.value < minInterval) {
        log(`Refresh took ${duration.toFixed(0)}ms — extending interval to ${minInterval}ms to avoid overlap`)
        setRefreshInterval(minInterval)
      }
    }
  }

  // Force refresh
  const forceRefresh = () => {
    log('Force refresh requested')
    performRefresh()
  }

  // Set transition states
  const setTransitionStates = (states) => {
    const hasTransitions = states && states.length > 0
    if (hasTransitions !== hasTransitionStates.value) {
      hasTransitionStates.value = hasTransitions
      log(`Transition states changed: ${hasTransitions}`)
    }
  }

  // Watch optimal interval changes
  watch(optimalInterval, (newInterval, oldInterval) => {
    if (newInterval !== oldInterval) {
      log(`Optimal interval changed from ${oldInterval}ms to ${newInterval}ms`)
      setRefreshInterval(newInterval)
    }
  })

  // Store event listeners for cleanup
  const eventListeners = ref({
    userActivity: null,
    activityChecker: null,
    networkOnline: null,
    networkOffline: null,
    visibilityChange: null
  })

  // Start smart refresh
  const start = () => {
    log('Starting smart refresh')
    
    // Set up event listeners
    if (enableUserActivity) {
      const events = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart', 'click']
      events.forEach(event => {
        document.addEventListener(event, updateUserActivity, { passive: true })
      })
      
      // Check user activity periodically
      eventListeners.value.activityChecker = setInterval(checkUserInactivity, 30000)
      
      // Store cleanup function
      eventListeners.value.userActivity = () => {
        events.forEach(event => {
          document.removeEventListener(event, updateUserActivity)
        })
        if (eventListeners.value.activityChecker) {
          clearInterval(eventListeners.value.activityChecker)
          eventListeners.value.activityChecker = null
        }
      }
    }

    if (enableNetworkDetection) {
      const onlineHandler = () => handleNetworkChange(true)
      const offlineHandler = () => handleNetworkChange(false)
      
      window.addEventListener('online', onlineHandler)
      window.addEventListener('offline', offlineHandler)
      
      // Store cleanup function
      eventListeners.value.networkOnline = () => window.removeEventListener('online', onlineHandler)
      eventListeners.value.networkOffline = () => window.removeEventListener('offline', offlineHandler)
    }

    if (enableVisibilityDetection) {
      document.addEventListener('visibilitychange', handleVisibilityChange)
      
      // Store cleanup function
      eventListeners.value.visibilityChange = () => {
        document.removeEventListener('visibilitychange', handleVisibilityChange)
      }
    }

    // Start refresh timer
    setRefreshInterval(optimalInterval.value)
    
    // Perform initial refresh
    performRefresh()
  }

  // Stop smart refresh
  const stop = () => {
    log('Stopping smart refresh')
    
    if (refreshTimer.value) {
      clearInterval(refreshTimer.value)
      refreshTimer.value = null
    }
    
    // Clean up event listeners
    Object.values(eventListeners.value).forEach(cleanup => {
      if (typeof cleanup === 'function') {
        cleanup()
      }
    })
    
    // Reset event listeners
    eventListeners.value = {
      userActivity: null,
      activityChecker: null,
      networkOnline: null,
      networkOffline: null,
      visibilityChange: null
    }
  }

  // Restart smart refresh
  const restart = () => {
    log('Restarting smart refresh')
    stop()
    start()
  }

  // Lifecycle management
  onMounted(() => {
    start()
  })

  onUnmounted(() => {
    stop()
  })

  return {
    // 状态
    isRefreshing: readonly(isRefreshing),
    errorCount: readonly(errorCount),
    lastRefreshTime: readonly(lastRefreshTime),
    currentInterval: readonly(currentInterval),
    isUserActive: readonly(isUserActive),
    isNetworkOnline: readonly(isNetworkOnline),
    isPageVisible: readonly(isPageVisible),
    
    // 方法
    forceRefresh,
    setTransitionStates,
    start,
    stop,
    restart,
    resetErrorCount,  // 导出resetErrorCount函数
    handleError,      // 导出handleError函数
    
    // 配置
    setBaseInterval: (interval) => {
      baseIntervalMs = interval
      log(`Base interval updated to ${interval}ms`)
    },
    setFastInterval: (interval) => {
      fastIntervalMs = interval
      log(`Fast interval updated to ${interval}ms`)
    }
  }
}

// 专门用于Dashboard的智能刷新
export function useDashboardSmartRefresh(refreshFunction, options = {}) {
  return useSmartRefresh(refreshFunction, {
    baseInterval: 60000,    // 1分钟
    fastInterval: 500,      // 0.5秒 (过渡状态快速刷新)
    slowInterval: 300000,   // 5分钟
    debug: true,
    ...options
  })
}

// 专门用于列表的智能刷新
export function useListSmartRefresh(refreshFunction, options = {}) {
  return useSmartRefresh(refreshFunction, {
    baseInterval: 300000,   // 5分钟
    fastInterval: 1000,     // 1秒
    slowInterval: 300000,   // 5分钟
    enableUserActivity: false, // 列表不需要用户活动检测
    debug: false,
    ...options
  })
}

// 专门用于实时数据的智能刷新
export function useRealtimeSmartRefresh(refreshFunction, options = {}) {
  return useSmartRefresh(refreshFunction, {
    baseInterval: 5000,     // 5秒
    fastInterval: 1000,     // 1秒
    slowInterval: 30000,    // 30秒
    enableVisibilityDetection: true,
    debug: false,
    ...options
  })
} 