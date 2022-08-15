// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React, {useEffect, useState, useMemo, useCallback} from 'react'
import {batch} from 'react-redux'
import {FormattedMessage, useIntl} from 'react-intl'
import {useRouteMatch} from 'react-router-dom'

import Workspace from '../../components/workspace'
import CloudMessage from '../../components/messages/cloudMessage'
import VersionMessage from '../../components/messages/versionMessage'
import octoClient from '../../octoClient'
import {Subscription, WSClient} from '../../wsclient'
import {Utils} from '../../utils'
import {useWebsockets} from '../../hooks/websockets'
import {IUser, UserConfigPatch} from '../../user'
import {Block} from '../../blocks/block'
import {ContentBlock} from '../../blocks/contentBlock'
import {CommentBlock} from '../../blocks/commentBlock'
import {Board, BoardMember} from '../../blocks/board'
import {BoardView} from '../../blocks/boardView'
import {Card} from '../../blocks/card'
import {
    updateBoards,
    updateMembersEnsuringBoardsAndUsers,
    getCurrentBoardId,
    setCurrent as setCurrentBoard,
    fetchBoardMembers,
    addMyBoardMemberships,
} from '../../store/boards'
import {getCurrentViewId, setCurrent as setCurrentView} from '../../store/views'
import {initialLoad, initialReadOnlyLoad, loadBoardData} from '../../store/initialLoad'
import {useAppSelector, useAppDispatch} from '../../store/hooks'
import {setTeam} from '../../store/teams'
import {updateViews} from '../../store/views'
import {updateCards} from '../../store/cards'
import {updateComments} from '../../store/comments'
import {updateContents} from '../../store/contents'
import {
    fetchUserBlockSubscriptions,
    getMe,
    followBlock,
    unfollowBlock, patchProps,
} from '../../store/users'
import {setGlobalError} from '../../store/globalError'
import {UserSettings} from '../../userSettings'

import IconButton from '../../widgets/buttons/iconButton'
import CloseIcon from '../../widgets/icons/close'

import TelemetryClient, {TelemetryActions, TelemetryCategory} from '../../telemetry/telemetryClient'

import {Constants} from "../../constants"

import SetWindowTitleAndIcon from './setWindowTitleAndIcon'
import TeamToBoardAndViewRedirect from './teamToBoardAndViewRedirect'
import UndoRedoHotKeys from './undoRedoHotKeys'
import BackwardCompatibilityQueryParamsRedirect from './backwardCompatibilityQueryParamsRedirect'
import WebsocketConnection from './websocketConnection'

import './boardPage.scss'

type Props = {
    readonly?: boolean
    new?: boolean
}

const BoardPage = (props: Props): JSX.Element => {
    const intl = useIntl()
    const activeBoardId = useAppSelector(getCurrentBoardId)
    const activeViewId = useAppSelector(getCurrentViewId)
    const dispatch = useAppDispatch()
    const match = useRouteMatch<{boardId: string, viewId: string, cardId?: string, teamId?: string}>()
    const [mobileWarningClosed, setMobileWarningClosed] = useState(UserSettings.mobileWarningClosed)
    const teamId = match.params.teamId || UserSettings.lastTeamId || Constants.globalTeamId
    const viewId = match.params.viewId
    const me = useAppSelector<IUser|null>(getMe)

    // if we're in a legacy route and not showing a shared board,
    // redirect to the new URL schema equivalent
    if (Utils.isFocalboardLegacy() && !props.readonly) {
        window.location.href = window.location.href.replace('/plugins/focalboard', '/boards')
    }

    // Load user's block subscriptions when workspace changes
    // block subscriptions are relevant only in plugin mode.
    if (Utils.isFocalboardPlugin()) {
        useEffect(() => {
            if (!me) {
                return
            }
            dispatch(fetchUserBlockSubscriptions(me!.id))
        }, [me?.id])
    }

    // TODO: Make this less brittle. This only works because this is the root render function
    useEffect(() => {
        UserSettings.lastTeamId = teamId
        octoClient.teamId = teamId
        dispatch(setTeam(teamId))
    }, [teamId])

    const loadAction: (boardId: string) => any = useMemo(() => {
        if (props.readonly) {
            return initialReadOnlyLoad
        }
        return initialLoad
    }, [props.readonly])

    useWebsockets(teamId, (wsClient) => {
        const incrementalBlockUpdate = (_: WSClient, blocks: Block[]) => {
            const teamBlocks = blocks

            batch(() => {
                dispatch(updateViews(teamBlocks.filter((b: Block) => b.type === 'view' || b.deleteAt !== 0) as BoardView[]))
                dispatch(updateCards(teamBlocks.filter((b: Block) => b.type === 'card' || b.deleteAt !== 0) as Card[]))
                dispatch(updateComments(teamBlocks.filter((b: Block) => b.type === 'comment' || b.deleteAt !== 0) as CommentBlock[]))
                dispatch(updateContents(teamBlocks.filter((b: Block) => b.type !== 'card' && b.type !== 'view' && b.type !== 'board' && b.type !== 'comment') as ContentBlock[]))
            })
        }

        const incrementalBoardUpdate = (_: WSClient, boards: Board[]) => {
            // only takes into account the entities that belong to the team or the user boards
            const teamBoards = boards.filter((b: Board) => b.teamId === Constants.globalTeamId || b.teamId === teamId)
            dispatch(updateBoards(teamBoards))
        }

        const incrementalBoardMemberUpdate = (_: WSClient, members: BoardMember[]) => {
            dispatch(updateMembersEnsuringBoardsAndUsers(members))

            if (me) {
                const myBoardMemberships = members.filter((boardMember) => boardMember.userId === me.id)
                dispatch(addMyBoardMemberships(myBoardMemberships))
            }
        }

        console.log('useWEbsocket adding onChange handler')
        wsClient.addOnChange(incrementalBlockUpdate, 'block')
        wsClient.addOnChange(incrementalBoardUpdate, 'board')
        wsClient.addOnChange(incrementalBoardMemberUpdate, 'boardMembers')
        wsClient.addOnReconnect(() => dispatch(loadAction(match.params.boardId)))

        wsClient.setOnFollowBlock((_: WSClient, subscription: Subscription): void => {
            if (subscription.subscriberId === me?.id) {
                dispatch(followBlock(subscription))
            }
        })
        wsClient.setOnUnfollowBlock((_: WSClient, subscription: Subscription): void => {
            if (subscription.subscriberId === me?.id) {
                dispatch(unfollowBlock(subscription))
            }
        })

        return () => {
            console.log('useWebsocket cleanup')
            wsClient.removeOnChange(incrementalBlockUpdate, 'block')
            wsClient.removeOnChange(incrementalBoardUpdate, 'board')
            wsClient.removeOnChange(incrementalBoardMemberUpdate, 'boardMembers')
            wsClient.removeOnReconnect(() => dispatch(loadAction(match.params.boardId)))
        }
    }, [me?.id])

    const loadOrJoinBoard = useCallback(async (userId: string, boardTeamId: string, boardId: string) => {
        // and fetch its data
        const result: any = await dispatch(loadBoardData(boardId))
        if (result.payload.blocks.length === 0 && userId) {
            const member = await octoClient.joinBoard(boardId)
            if (!member) {
                UserSettings.setLastBoardID(boardTeamId, null)
                UserSettings.setLastViewId(boardId, null)
                dispatch(setGlobalError('board-not-found'))
                return
            }
            await dispatch(loadBoardData(boardId))
        }

        dispatch(fetchBoardMembers({
            teamId: boardTeamId,
            boardId,
        }))
    }, [])

    useEffect(() => {
        dispatch(loadAction(match.params.boardId))

        if (match.params.boardId) {
            // set the active board
            dispatch(setCurrentBoard(match.params.boardId))

            // and set it as most recently viewed board
            UserSettings.setLastBoardID(teamId, match.params.boardId)

            if (viewId && viewId !== Constants.globalTeamId) {
                dispatch(setCurrentView(viewId))
                UserSettings.setLastViewId(match.params.boardId, viewId)
            }

            if (!props.readonly && me) {
                loadOrJoinBoard(me.id, teamId, match.params.boardId)
            }
        }
    }, [teamId, match.params.boardId, viewId, me?.id])

    const handleUnhideBoard = async (boardID: string) => {
        console.log(`handleUnhideBoard called`)
        if (!me) {
            return
        }

        const hiddenBoards = {...(me.props.hiddenBoardIDs || {})}
        // const index = hiddenBoards.indexOf(boardID)
        // hiddenBoards.splice(index, 1)
        delete hiddenBoards[boardID]
        const hiddenBoardsArray = Object.keys(hiddenBoards)
        const patch: UserConfigPatch = {
            updatedFields: {
                'hiddenBoardIDs': JSON.stringify(hiddenBoardsArray),
            }
        }
        const patchedProps = await octoClient.patchUserConfig(me.id, patch)
        if (!patchedProps) {
            return
        }

        await dispatch(patchProps(patchedProps))
    }

    useEffect(() => {
        if (!teamId || !match.params.boardId) {
            return
        }

        const hiddenBoardIDs = me?.props.hiddenBoardIDs || {}
        if (hiddenBoardIDs[match.params.boardId]) {
            handleUnhideBoard(match.params.boardId)
        }
    }, [me?.id, teamId, match.params.boardId])

    if (props.readonly) {
        useEffect(() => {
            if (activeBoardId && activeViewId) {
                TelemetryClient.trackEvent(TelemetryCategory, TelemetryActions.ViewSharedBoard, {board: activeBoardId, view: activeViewId})
            }
        }, [activeBoardId, activeViewId])
    }

    return (
        <div className='BoardPage'>
            {!props.new && <TeamToBoardAndViewRedirect/>}
            <BackwardCompatibilityQueryParamsRedirect/>
            <SetWindowTitleAndIcon/>
            <UndoRedoHotKeys/>
            <WebsocketConnection/>
            <CloudMessage/>
            <VersionMessage/>

            {!mobileWarningClosed &&
                <div className='mobileWarning'>
                    <div>
                        <FormattedMessage
                            id='Error.mobileweb'
                            defaultMessage='Mobile web support is currently in early beta. Not all functionality may be present.'
                        />
                    </div>
                    <IconButton
                        onClick={() => {
                            UserSettings.mobileWarningClosed = true
                            setMobileWarningClosed(true)
                        }}
                        icon={<CloseIcon/>}
                        title='Close'
                        className='margin-right'
                    />
                </div>}

            {props.readonly && activeBoardId === undefined &&
                <div className='error'>
                    {intl.formatMessage({id: 'BoardPage.syncFailed', defaultMessage: 'Board may be deleted or access revoked.'})}
                </div>}

            {
                // Don't display Templates page
                // if readonly mode and no board defined.
                (!props.readonly || activeBoardId !== undefined) &&
                <Workspace
                    readonly={props.readonly || false}
                />
            }
        </div>
    )
}

export default BoardPage
