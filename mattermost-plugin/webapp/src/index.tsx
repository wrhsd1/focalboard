// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React, {useEffect} from 'react'
import {Store, Action} from 'redux'
import {Provider as ReduxProvider} from 'react-redux'
import {createBrowserHistory, History} from 'history'

import {rudderAnalytics, RudderTelemetryHandler} from 'mattermost-redux/client/rudder'

import {GlobalState} from 'mattermost-redux/types/store'

import {selectTeam} from 'mattermost-redux/actions/teams'

import {SuiteWindow} from '../../../webapp/src/types/index'
import {UserSettings} from '../../../webapp/src/userSettings'


const windowAny = (window as SuiteWindow)
windowAny.baseURL = '/plugins/focalboard'
windowAny.frontendBaseURL = '/boards'
windowAny.isFocalboardPlugin = true

import App from '../../../webapp/src/app'
import store from '../../../webapp/src/store'
import {setTeam} from '../../../webapp/src/store/teams'
import WithWebSockets from '../../../webapp/src/components/withWebSockets'
import {setChannel} from '../../../webapp/src/store/channels'
import {initialLoad} from '../../../webapp/src/store/initialLoad'
import {Utils} from '../../../webapp/src/utils'
import GlobalHeader from '../../../webapp/src/components/globalHeader/globalHeader'
import FocalboardIcon from '../../../webapp/src/widgets/icons/logo'
import {setMattermostTheme} from '../../../webapp/src/theme'

import TelemetryClient, {TelemetryCategory, TelemetryActions} from '../../../webapp/src/telemetry/telemetryClient'

import '../../../webapp/src/styles/focalboard-variables.scss'
import '../../../webapp/src/styles/main.scss'
import '../../../webapp/src/styles/labels.scss'
import octoClient from '../../../webapp/src/octoClient'
import {Constants} from '../../../webapp/src/constants'

import BoardsUnfurl from './components/boardsUnfurl/boardsUnfurl'
import RHSChannelBoards from './components/rhsChannelBoards'
import RHSChannelBoardsHeader from './components/rhsChannelBoardsHeader'
import BoardSelector from './components/boardSelector'
import wsClient, {
    MMWebSocketClient,
    ACTION_UPDATE_BLOCK,
    ACTION_UPDATE_CLIENT_CONFIG,
    ACTION_UPDATE_SUBSCRIPTION,
    ACTION_UPDATE_CARD_LIMIT_TIMESTAMP,
    ACTION_UPDATE_CATEGORY,
    ACTION_UPDATE_BOARD_CATEGORY,
    ACTION_UPDATE_BOARD,
} from './../../../webapp/src/wsclient'

import manifest from './manifest'
import ErrorBoundary from './error_boundary'

// eslint-disable-next-line import/no-unresolved
import {PluginRegistry} from './types/mattermost-webapp'

import './plugin.scss'
import CloudUpgradeNudge from "./components/cloudUpgradeNudge/cloudUpgradeNudge"

function getSubpath(siteURL: string): string {
    const url = new URL(siteURL)

    // remove trailing slashes
    return url.pathname.replace(/\/+$/, '')
}

const TELEMETRY_RUDDER_KEY = 'placeholder_rudder_key'
const TELEMETRY_RUDDER_DATAPLANE_URL = 'placeholder_rudder_dataplane_url'
const TELEMETRY_OPTIONS = {
    context: {
        ip: '0.0.0.0',
    },
    page: {
        path: '',
        referrer: '',
        search: '',
        title: '',
        url: '',
    },
    anonymousId: '00000000000000000000000000',
}

type Props = {
    webSocketClient: MMWebSocketClient
}

function customHistory() {
    const history = createBrowserHistory({basename: Utils.getFrontendBaseURL()})

    if (Utils.isDesktop()) {
        window.addEventListener('message', (event: MessageEvent) => {
            if (event.origin !== windowAny.location.origin) {
                return
            }

            const pathName = event.data.message?.pathName
            if (!pathName || !pathName.startsWith('/boards')) {
                return
            }

            Utils.log(`Navigating Boards to ${pathName}`)
            history.replace(pathName.replace('/boards', ''))
        })
    }
    return {
        ...history,
        push: (path: string, state?: unknown) => {
            if (Utils.isDesktop()) {
                windowAny.postMessage(
                    {
                        type: 'browser-history-push',
                        message: {
                            path: `${windowAny.frontendBaseURL}${path}`,
                        },
                    },
                    windowAny.location.origin,
                )
            } else {
                history.push(path, state as Record<string, never>)
            }
        },
    }
}

let browserHistory: History<unknown>

const MainApp = (props: Props) => {
    useEffect(() => {
        document.body.classList.add('focalboard-body')
        document.body.classList.add('app__body')
        const root = document.getElementById('root')
        if (root) {
            root.classList.add('focalboard-plugin-root')
        }

        return () => {
            document.body.classList.remove('focalboard-body')
            document.body.classList.remove('app__body')
            if (root) {
                root.classList.remove('focalboard-plugin-root')
            }
        }
    }, [])

    return (
        <ErrorBoundary>
            <ReduxProvider store={store}>
                <WithWebSockets manifest={manifest} webSocketClient={props.webSocketClient}>
                    <div id='focalboard-app'>
                        <App history={browserHistory}/>
                    </div>
                    <div id='focalboard-root-portal'/>
                </WithWebSockets>
            </ReduxProvider>
        </ErrorBoundary>
    )
}

const HeaderComponent = () => {
    return (
        <ErrorBoundary>
            <GlobalHeader history={browserHistory}/>
        </ErrorBoundary>
    )
}

export default class Plugin {
    channelHeaderButtonId?: string
    rhsId?: string
    boardSelectorId?: string
    registry?: PluginRegistry

    // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-function
    async initialize(registry: PluginRegistry, mmStore: Store<GlobalState, Action<Record<string, unknown>>>): Promise<void> {
        const siteURL = mmStore.getState().entities.general.config.SiteURL
        const subpath = siteURL ? getSubpath(siteURL) : ''
        windowAny.frontendBaseURL = subpath + windowAny.frontendBaseURL
        windowAny.baseURL = subpath + windowAny.baseURL
        browserHistory = customHistory()

        this.registry = registry

        UserSettings.nameFormat = mmStore.getState().entities.preferences?.myPreferences['display_settings--name_format']?.value || null
        let theme = mmStore.getState().entities.preferences.myPreferences.theme
        setMattermostTheme(theme)

        // register websocket handlers
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_BOARD}`, (e: any) => wsClient.updateHandler(e.data))
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_CATEGORY}`, (e: any) => wsClient.updateHandler(e.data))
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_BOARD_CATEGORY}`, (e: any) => wsClient.updateHandler(e.data))
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_CLIENT_CONFIG}`, (e: any) => wsClient.updateClientConfigHandler(e.data))
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_CARD_LIMIT_TIMESTAMP}`, (e: any) => wsClient.updateCardLimitTimestampHandler(e.data))
        this.registry?.registerWebSocketEventHandler(`custom_${manifest.id}_${ACTION_UPDATE_SUBSCRIPTION}`, (e: any) => wsClient.updateSubscriptionHandler(e.data))
        this.registry?.registerWebSocketEventHandler('plugin_statuses_changed', (e: any) => wsClient.pluginStatusesChangedHandler(e.data))
        this.registry?.registerPostTypeComponent('custom_cloud_upgrade_nudge', CloudUpgradeNudge)
        this.registry?.registerWebSocketEventHandler('preferences_changed', (e: any) => {
            let preferences
            try {
                preferences = JSON.parse(e.data.preferences)
            } catch {
                preferences = []
            }
            if (preferences) {
                for (const preference of preferences) {
                    if (preference.category === 'theme' && theme !== preference.value) {
                        setMattermostTheme(JSON.parse(preference.value))
                        theme = preference.value
                    }
                    if(preference.category === 'display_settings' && preference.name === 'name_format'){
                        UserSettings.nameFormat = preference.value
                    }
                }
            }
        })

        let lastViewedChannel = mmStore.getState().entities.channels.currentChannelId
        let prevTeamID: string

        const currentChannel = mmStore.getState().entities.channels.currentChannelId
        const currentChannelObj = mmStore.getState().entities.channels.channels[currentChannel]
        store.dispatch(setChannel(currentChannelObj))

        mmStore.subscribe(() => {
            const currentUserId = mmStore.getState().entities.users.currentUserId
            const currentChannel = mmStore.getState().entities.channels.currentChannelId
            if (lastViewedChannel !== currentChannel && currentChannel) {
                localStorage.setItem('focalboardLastViewedChannel:' + currentUserId, currentChannel)
                lastViewedChannel = currentChannel
                const currentChannelObj = mmStore.getState().entities.channels.channels[lastViewedChannel]
                store.dispatch(setChannel(currentChannelObj))
            }

            // Watch for change in active team.
            // This handles the user selecting a team from the team sidebar.
            const currentTeamID = mmStore.getState().entities.teams.currentTeamId
            if (currentTeamID && currentTeamID !== prevTeamID) {
                if (prevTeamID && window.location.pathname.startsWith(windowAny.frontendBaseURL || '')) {
                    // Don't re-push the URL if we're already on a URL for the current team
                    if (!window.location.pathname.startsWith(`${(windowAny.frontendBaseURL || '')}/team/${currentTeamID}`))
                        browserHistory.push(`/team/${currentTeamID}`)
                }
                prevTeamID = currentTeamID
                store.dispatch(setTeam(currentTeamID))
                octoClient.teamId = currentTeamID
                store.dispatch(initialLoad())
            }
        })

        let fbPrevTeamID = store.getState().teams.currentId
        store.subscribe(() => {
            const currentTeamID: string = store.getState().teams.currentId
            const currentUserId = mmStore.getState().entities.users.currentUserId
            if (currentTeamID !== fbPrevTeamID) {
                fbPrevTeamID = currentTeamID
                // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                // @ts-ignore
                mmStore.dispatch(selectTeam(currentTeamID))
                localStorage.setItem(`user_prev_team:${currentUserId}`, currentTeamID)
            }
        })

        if (this.registry.registerProduct) {
            windowAny.frontendBaseURL = subpath + '/boards'

            const {rhsId, toggleRHSPlugin} = this.registry.registerRightHandSidebarComponent(
                (props: {webSocketClient: MMWebSocketClient}) => (
                    <ReduxProvider store={store}>
                        <WithWebSockets manifest={manifest} webSocketClient={props.webSocketClient}>
                            <RHSChannelBoards/>
                        </WithWebSockets>
                    </ReduxProvider>
                ),
                <ErrorBoundary>
                    <ReduxProvider store={store}>
                        <RHSChannelBoardsHeader/>
                    </ReduxProvider>
                </ErrorBoundary>
                ,
            )
            this.rhsId = rhsId

            this.channelHeaderButtonId = registry.registerChannelHeaderButtonAction(<FocalboardIcon/>, () => mmStore.dispatch(toggleRHSPlugin), 'Boards', 'Boards')

            this.registry.registerProduct(
                '/boards',
                'product-boards',
                'Boards',
                '/boards',
                MainApp,
                HeaderComponent,
                () => null,
                true,
            )

            const goToFocalboardTemplate = () => {
                const currentTeam = mmStore.getState().entities.teams.currentTeamId
                const currentChannel = mmStore.getState().entities.channels.currentChannelId
                TelemetryClient.trackEvent(TelemetryCategory, TelemetryActions.ClickChannelIntro, {teamID: currentTeam})
                window.open(`${windowAny.frontendBaseURL}/team/${currentTeam}/new/${currentChannel}`, '_blank', 'noopener')
            }

            if (registry.registerChannelIntroButtonAction) {
                this.channelHeaderButtonId = registry.registerChannelIntroButtonAction(<FocalboardIcon/>, goToFocalboardTemplate, 'Boards')
            }

            if (this.registry.registerAppBarComponent) {
                const appBarIconURL = windowAny.baseURL + '/public/app-bar-icon.png'
                this.registry.registerAppBarComponent(appBarIconURL, () => mmStore.dispatch(toggleRHSPlugin), 'Boards')
            }

            this.registry.registerPostWillRenderEmbedComponent(
                (embed) => embed.type === 'boards',
                (props: {embed: {data: string}, webSocketClient: MMWebSocketClient}) => (
                    <ReduxProvider store={store}>
                        <BoardsUnfurl
                            embed={props.embed}
                            webSocketClient={props.webSocketClient}
                        />
                    </ReduxProvider>
                ),
                false
            )

            // Insights handler
            if (this.registry?.registerInsightsHandler) {
                this.registry?.registerInsightsHandler(async (timeRange: string, page: number, perPage: number, teamId: string, insightType: string) => {
                    if (insightType === Constants.myInsights) {
                        const data = await octoClient.getMyTopBoards(timeRange, page, perPage, teamId)

                        return data
                    } 

                    const data = await octoClient.getTeamTopBoards(timeRange, page, perPage, teamId)

                    return data
                })
            }
        }

        this.boardSelectorId = this.registry.registerRootComponent((props: {webSocketClient: MMWebSocketClient}) => (
            <ReduxProvider store={store}>
                <WithWebSockets manifest={manifest} webSocketClient={props.webSocketClient}>
                    <BoardSelector/>
                </WithWebSockets>
            </ReduxProvider>
        ))

        const config = await octoClient.getClientConfig()
        if (config?.telemetry) {
            let rudderKey = TELEMETRY_RUDDER_KEY
            let rudderUrl = TELEMETRY_RUDDER_DATAPLANE_URL

            if (rudderKey.startsWith('placeholder') && rudderUrl.startsWith('placeholder')) {
                rudderKey = process.env.RUDDER_KEY as string //eslint-disable-line no-process-env
                rudderUrl = process.env.RUDDER_DATAPLANE_URL as string //eslint-disable-line no-process-env
            }

            if (rudderKey !== '') {
                const rudderCfg = {} as {setCookieDomain: string}
                if (siteURL && siteURL !== '') {
                    try {
                        rudderCfg.setCookieDomain = new URL(siteURL).hostname
                        // eslint-disable-next-line no-empty
                    } catch (_) {}
                }
                rudderAnalytics.load(rudderKey, rudderUrl, rudderCfg)

                rudderAnalytics.identify(config?.telemetryid, {}, TELEMETRY_OPTIONS)

                rudderAnalytics.page('BoardsLoaded', '',
                    TELEMETRY_OPTIONS.page,
                    {
                        context: TELEMETRY_OPTIONS.context,
                        anonymousId: TELEMETRY_OPTIONS.anonymousId,
                    })

                rudderAnalytics.ready(() => {
                    TelemetryClient.setTelemetryHandler(new RudderTelemetryHandler())
                })
            }
        }

        windowAny.getCurrentTeamId = (): string => {
            // eslint-disable-next-line @typescript-eslint/ban-ts-comment
            // @ts-ignore
            return mmStore.getState().entities.teams.currentTeamId
        }
    }

    uninitialize(): void {
        if (this.channelHeaderButtonId) {
            this.registry?.unregisterComponent(this.channelHeaderButtonId)
        }
        if (this.rhsId) {
            this.registry?.unregisterComponent(this.rhsId)
        }
        if (this.boardSelectorId) {
            this.registry?.unregisterComponent(this.boardSelectorId)
        }

        // unregister websocket handlers
        this.registry?.unregisterWebSocketEventHandler(wsClient.clientPrefix + ACTION_UPDATE_BLOCK)
    }
}

declare global {
    interface Window {
        registerPlugin(id: string, plugin: Plugin): void
    }
}

window.registerPlugin(manifest.id, new Plugin())
