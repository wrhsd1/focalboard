// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {createSlice, createAsyncThunk, PayloadAction, createSelector} from '@reduxjs/toolkit'

import {default as client} from '../octoClient'
import {IUser, parseUserProps} from '../user'

import {Utils} from '../utils'

import {Subscription} from '../wsclient'

// TODO: change this whene the initial load is complete
// import {initialLoad} from './initialLoad'
import {UserSettings} from '../userSettings'

import {RootState} from './index'

export const fetchMe = createAsyncThunk(
    'users/fetchMe',
    async () => client.getMe(),
)

export const versionProperty = 'focalboard_version72MessageCanceled'

type UsersStatus = {
    me: IUser|null
    boardUsers: {[key: string]: IUser}
    loggedIn: boolean|null
    blockSubscriptions: Array<Subscription>
}

export const fetchUserBlockSubscriptions = createAsyncThunk(
    'user/blockSubscriptions',
    async (userId: string) => (Utils.isFocalboardPlugin() ? client.getUserBlockSubscriptions(userId) : []),
)

const initialState = {
    me: null,
    boardUsers: {},
    loggedIn: null,
    userWorkspaces: [],
    blockSubscriptions: [],
} as UsersStatus

const usersSlice = createSlice({
    name: 'users',
    initialState,
    reducers: {
        setMe: (state, action: PayloadAction<IUser|null>) => {
            state.me = action.payload
            if (state.me) {
                state.me.props = parseUserProps(state.me.props)
            }
            state.loggedIn = Boolean(state.me)
        },
        setBoardUsers: (state, action: PayloadAction<IUser[]>) => {
            state.boardUsers = action.payload.reduce((acc: {[key: string]: IUser}, user: IUser) => {
                acc[user.id] = user
                return acc
            }, {})
        },
        addBoardUsers: (state, action: PayloadAction<IUser[]>) => {
            action.payload.forEach((user: IUser) => {
                state.boardUsers[user.id] = user
            })
        },
        removeBoardUsersById: (state, action: PayloadAction<string[]>) => {
            action.payload.forEach((userId: string) => {
                delete state.boardUsers[userId]
            })
        },
        followBlock: (state, action: PayloadAction<Subscription>) => {
            state.blockSubscriptions.push(action.payload)
        },
        unfollowBlock: (state, action: PayloadAction<Subscription>) => {
            const oldSubscriptions = state.blockSubscriptions
            state.blockSubscriptions = oldSubscriptions.filter((subscription) => subscription.blockId !== action.payload.blockId)
        },
        patchProps: (state, action: PayloadAction<Record<string, string>>) => {
            if (state.me) {
                state.me.props = parseUserProps(action.payload)
            }
        },
    },
    extraReducers: (builder) => {
        builder.addCase(fetchMe.fulfilled, (state, action) => {
            state.me = action.payload || null
            if (state.me) {
                state.me.props = parseUserProps(state.me.props)
            }
            state.loggedIn = Boolean(state.me)
        })
        builder.addCase(fetchMe.rejected, (state) => {
            state.me = null
            state.loggedIn = false
        })

        // TODO: change this when the initial load is complete
        // builder.addCase(initialLoad.fulfilled, (state, action) => {
        //     state.boardUsers = action.payload.boardUsers.reduce((acc: {[key: string]: IUser}, user: IUser) => {
        //         acc[user.id] = user
        //         return acc
        //     }, {})
        // })

        builder.addCase(fetchUserBlockSubscriptions.fulfilled, (state, action) => {
            state.blockSubscriptions = action.payload
        })
    },
})

export const {setMe, setBoardUsers, removeBoardUsersById, addBoardUsers, followBlock, unfollowBlock, patchProps} = usersSlice.actions
export const {reducer} = usersSlice

export const getMe = (state: RootState): IUser|null => state.users.me
export const getLoggedIn = (state: RootState): boolean|null => state.users.loggedIn
export const getBoardUsers = (state: RootState): {[key: string]: IUser} => state.users.boardUsers

export const getBoardUsersList = createSelector(
    getBoardUsers,
    (boardUsers) => Object.values(boardUsers).sort((a, b) => a.username.localeCompare(b.username)),
)

export const getUser = (userId: string): (state: RootState) => IUser|undefined => {
    return (state: RootState): IUser|undefined => {
        const users = getBoardUsers(state)
        return users[userId]
    }
}

export const getOnboardingTourStarted = createSelector(
    getMe,
    (me): boolean => {
        if (!me) {
            return false
        }

        return Boolean(me.props?.focalboard_onboardingTourStarted)
    },
)

export const getOnboardingTourStep = createSelector(
    getMe,
    (me): string => {
        if (!me) {
            return ''
        }

        return me.props?.focalboard_onboardingTourStep
    },
)

export const getOnboardingTourCategory = createSelector(
    getMe,
    (me): string => (me ? me.props?.focalboard_tourCategory : ''),
)

export const getCloudMessageCanceled = createSelector(
    getMe,
    (me): boolean => {
        if (!me) {
            return false
        }
        if (me.id === 'single-user') {
            return UserSettings.hideCloudMessage
        }
        return Boolean(me.props?.focalboard_cloudMessageCanceled)
    },
)

export const getVersionMessageCanceled = createSelector(
    getMe,
    (me): boolean => {
        if (versionProperty && me){
            if (me.id === 'single-user') {
                return true
            }
            return Boolean(me.props[versionProperty])    
        }
        return true
    },
)

export const getCardLimitSnoozeUntil = createSelector(
    getMe,
    (me): number => {
        if (!me) {
            return 0
        }
        try {
            return parseInt(me.props?.focalboard_cardLimitSnoozeUntil, 10) || 0
        } catch (_) {
            return 0
        }
    },
)

export const getCardHiddenWarningSnoozeUntil = createSelector(
    getMe,
    (me): number => {
        if (!me) {
            return 0
        }
        try {
            return parseInt(me.props?.focalboard_cardHiddenWarningSnoozeUntil, 10) || 0
        } catch (_) {
            return 0
        }
    },
)
