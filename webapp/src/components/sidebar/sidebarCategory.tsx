// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {FormattedMessage, useIntl} from 'react-intl'
import {generatePath, useHistory, useRouteMatch} from 'react-router-dom'

import {debounce} from "lodash"

import {Board} from '../../blocks/board'
import mutator from '../../mutator'
import IconButton from '../../widgets/buttons/iconButton'
import DeleteIcon from '../../widgets/icons/delete'
import OptionsIcon from '../../widgets/icons/options'
import Menu from '../../widgets/menu'
import MenuWrapper from '../../widgets/menuWrapper'

import './sidebarCategory.scss'
import {Category, CategoryBoards} from '../../store/sidebar'
import ChevronDown from '../../widgets/icons/chevronDown'
import ChevronRight from '../../widgets/icons/chevronRight'
import CreateNewFolder from '../../widgets/icons/newFolder'
import CreateCategory from '../createCategory/createCategory'
import {useAppSelector} from '../../store/hooks'
import {IUser} from '../../user'
import {
    getMe,
    getOnboardingTourCategory,
    getOnboardingTourStep,
} from '../../store/users'

import {getCurrentCard} from '../../store/cards'
import {Utils} from '../../utils'
import Update from '../../widgets/icons/update'

import { TOUR_SIDEBAR, SidebarTourSteps, TOUR_BOARD, FINISHED } from '../../components/onboardingTour/index'
import telemetryClient, {TelemetryActions, TelemetryCategory} from '../../telemetry/telemetryClient'

import {getCurrentTeam} from '../../store/teams'

import ConfirmationDialogBox, {ConfirmationDialogBoxProps} from '../confirmationDialogBox'

import SidebarCategoriesTourStep from '../../components/onboardingTour/sidebarCategories/sidebarCategories'
import ManageCategoriesTourStep from '../../components/onboardingTour/manageCategories/manageCategories'

import DeleteBoardDialog from './deleteBoardDialog'
import SidebarBoardItem from './sidebarBoardItem'

type Props = {
    activeCategoryId?: string
    activeBoardID?: string
    hideSidebar: () => void
    categoryBoards: CategoryBoards
    boards: Board[]
    allCategories: Array<CategoryBoards>
    index: number
}

export const ClassForManageCategoriesTourStep = 'manageCategoriesTourStep'

const SidebarCategory = (props: Props) => {
    const [collapsed, setCollapsed] = useState(props.categoryBoards.collapsed)
    const intl = useIntl()
    const history = useHistory()

    const [deleteBoard, setDeleteBoard] = useState<Board|null>()
    const [showDeleteCategoryDialog, setShowDeleteCategoryDialog] = useState<boolean>(false)
    const [categoryMenuOpen, setCategoryMenuOpen] = useState<boolean>(false)

    const match = useRouteMatch<{boardId: string, viewId?: string, cardId?: string, teamId?: string}>()
    const [showCreateCategoryModal, setShowCreateCategoryModal] = useState(false)
    const [showUpdateCategoryModal, setShowUpdateCategoryModal] = useState(false)
    const me = useAppSelector<IUser|null>(getMe)

    const onboardingTourCategory = useAppSelector(getOnboardingTourCategory)
    const onboardingTourStep = useAppSelector(getOnboardingTourStep)
    const currentCard = useAppSelector(getCurrentCard)
    const noCardOpen = !currentCard
    const team = useAppSelector(getCurrentTeam)
    const teamID = team?.id || ''

    const menuWrapperRef = useRef<HTMLDivElement>(null)

    const shouldViewSidebarTour = props.boards.length !== 0 &&
                                  noCardOpen &&
                                  (onboardingTourCategory === TOUR_SIDEBAR || onboardingTourCategory === TOUR_BOARD) &&
                                  ((onboardingTourCategory === TOUR_SIDEBAR && onboardingTourStep === SidebarTourSteps.SIDE_BAR.toString()) || (onboardingTourCategory === TOUR_BOARD && onboardingTourStep === FINISHED.toString()))

    const shouldViewManageCatergoriesTour = props.boards.length !== 0 &&
                                            noCardOpen &&
                                            onboardingTourCategory === TOUR_SIDEBAR &&
                                            onboardingTourStep === SidebarTourSteps.MANAGE_CATEGORIES.toString()

    useEffect(() => {
        if(shouldViewManageCatergoriesTour && props.index === 0) {
            setCategoryMenuOpen(true)
        }
    }, [shouldViewManageCatergoriesTour])

    const showBoard = useCallback((boardId) => {
        Utils.showBoard(boardId, match, history)
        props.hideSidebar()
    }, [match, history])

    const showView = useCallback((viewId, boardId) => {
        // if the same board, reuse the match params
        // otherwise remove viewId and cardId, results in first view being selected
        const params = {...match.params, boardId: boardId || '', viewId: viewId || ''}
        if (boardId !== match.params.boardId && viewId !== match.params.viewId) {
            params.cardId = undefined
        }
        const newPath = generatePath(Utils.getBoardPagePath(match.path), params)
        history.push(newPath)
        props.hideSidebar()
    }, [match, history])

    const isBoardVisible = (boardID: string): boolean => {
        // hide if board doesn't belong to current category
        if (!blocks.includes(boardID)) {
            return false
        }

        // hide if board was hidden by the user
        const hiddenBoardIDs = me?.props.hiddenBoardIDs || {}
        return !hiddenBoardIDs[boardID]
    }

    const blocks = props.categoryBoards.boardIDs || []
    const visibleBlocks = props.categoryBoards.boardIDs.filter((boardID) => isBoardVisible(boardID))

    const handleCreateNewCategory = () => {
        setShowCreateCategoryModal(true)
    }

    const handleDeleteCategory = async () => {
        await mutator.deleteCategory(teamID, props.categoryBoards.id)
    }

    const handleUpdateCategory = async () => {
        setShowUpdateCategoryModal(true)
    }

    const deleteCategoryProps: ConfirmationDialogBoxProps = {
        heading: intl.formatMessage({
            id: 'SidebarCategories.CategoryMenu.DeleteModal.Title',
            defaultMessage: 'Delete this category?',
        }),
        subText: intl.formatMessage(
            {
                id: 'SidebarCategories.CategoryMenu.DeleteModal.Body',
                defaultMessage: 'Boards in <b>{categoryName}</b> will move back to the Boards categories. You\'re not removed from any boards.',
            },
            {
                categoryName: props.categoryBoards.name,
                b: (...chunks) => <b>{chunks}</b>,
            },
        ),
        onConfirm: () => handleDeleteCategory(),
        onClose: () => setShowDeleteCategoryDialog(false),
    }

    const onDeleteBoard = useCallback(async () => {
        if (!deleteBoard) {
            return
        }
        telemetryClient.trackEvent(TelemetryCategory, TelemetryActions.DeleteBoard, {board: deleteBoard.id})
        mutator.deleteBoard(
            deleteBoard,
            intl.formatMessage({id: 'Sidebar.delete-board', defaultMessage: 'Delete board'}),
            async () => {
                let nextBoardId: number | undefined
                if (props.boards.length > 1) {
                    const deleteBoardIndex = props.boards.findIndex((board) => board.id === deleteBoard.id)
                    nextBoardId = deleteBoardIndex + 1 === props.boards.length ? deleteBoardIndex - 1 : deleteBoardIndex + 1
                }

                if (nextBoardId) {
                // This delay is needed because WSClient has a default 100 ms notification delay before updates
                    setTimeout(() => {
                        showBoard(props.boards[nextBoardId as number].id)
                    }, 120)
                }
            },
            async () => {
                showBoard(deleteBoard.id)
            },
        )
    }, [showBoard, deleteBoard, props.boards])

    const updateCategory = useCallback(async (value: boolean) => {
        const updatedCategory: Category = {
            ...props.categoryBoards,
            collapsed: value,
        }
        await mutator.updateCategory(updatedCategory)
    }, [props.categoryBoards])

    const debouncedUpdateCategory = useMemo(() => debounce(updateCategory, 400), [updateCategory])

    const toggleCollapse = async () => {
        const newVal = !collapsed
        await setCollapsed(newVal)

        // The default 'Boards' category isn't stored in database,
        // so avoid making the API call for it
        if (props.categoryBoards.id !== '') {
            debouncedUpdateCategory(newVal)
        }
    }

    return (
        <div className='SidebarCategory' ref={menuWrapperRef}>
            <div
                className={`octo-sidebar-item category ' ${collapsed ? 'collapsed' : 'expanded'} ${props.categoryBoards.id === props.activeCategoryId ? 'active' : ''}`}
            >
                <div
                    className='octo-sidebar-title category-title'
                    title={props.categoryBoards.name}
                    onClick={toggleCollapse}
                >
                    {collapsed ? <ChevronRight/> : <ChevronDown/>}
                    {props.categoryBoards.name}
                    <div className='sidebarCategoriesTour'>
                        {props.index === 0 && shouldViewSidebarTour && <SidebarCategoriesTourStep/>}
                    </div>
                </div>
                <div className={(props.index === 0 && shouldViewManageCatergoriesTour) ? `${ClassForManageCategoriesTourStep}` : ''}>
                    {props.index === 0 && shouldViewManageCatergoriesTour && <ManageCategoriesTourStep/>}
                    <MenuWrapper
                        className={categoryMenuOpen ? 'menuOpen' : ''}
                        stopPropagationOnToggle={true}
                        onToggle={(open) => setCategoryMenuOpen(open)}
                    >
                        <IconButton icon={<OptionsIcon/>}/>
                        <Menu
                            position='auto'
                            parentRef={menuWrapperRef}
                        >
                            <Menu.Text
                                id='createNewCategory'
                                name={intl.formatMessage({id: 'SidebarCategories.CategoryMenu.CreateNew', defaultMessage: 'Create New Category'})}
                                icon={<CreateNewFolder/>}
                                onClick={handleCreateNewCategory}
                            />
                            {
                                props.categoryBoards.id !== '' &&
                                <React.Fragment>
                                    <Menu.Text
                                        id='deleteCategory'
                                        className='text-danger'
                                        name={intl.formatMessage({id: 'SidebarCategories.CategoryMenu.Delete', defaultMessage: 'Delete Category'})}
                                        icon={<DeleteIcon/>}
                                        onClick={() => setShowDeleteCategoryDialog(true)}
                                    />
                                    <Menu.Text
                                        id='updateCategory'
                                        name={intl.formatMessage({id: 'SidebarCategories.CategoryMenu.Update', defaultMessage: 'Rename Category'})}
                                        icon={<Update/>}
                                        onClick={handleUpdateCategory}
                                    />
                                </React.Fragment>
                            }
                        </Menu>
                    </MenuWrapper>
                </div>
            </div>
            {!collapsed && visibleBlocks.length === 0 &&
                <div className='octo-sidebar-item subitem no-views'>
                    <FormattedMessage
                        id='Sidebar.no-boards-in-category'
                        defaultMessage='No boards inside'
                    />
                </div>}
            {collapsed && props.boards.filter((board: Board) => board.id === props.activeBoardID).map((board: Board) => {
                if (!isBoardVisible(board.id)) {
                    return null
                }
                return (
                    <SidebarBoardItem
                        key={board.id}
                        board={board}
                        categoryBoards={props.categoryBoards}
                        allCategories={props.allCategories}
                        isActive={board.id === props.activeBoardID}
                        showBoard={showBoard}
                        showView={showView}
                        onDeleteRequest={setDeleteBoard}
                    />
                )
            })}
            {!collapsed && props.boards.map((board: Board) => {
                if (!isBoardVisible(board.id)) {
                    return null
                }
                return (
                    <SidebarBoardItem
                        key={board.id}
                        board={board}
                        categoryBoards={props.categoryBoards}
                        allCategories={props.allCategories}
                        isActive={board.id === props.activeBoardID}
                        showBoard={showBoard}
                        showView={showView}
                        onDeleteRequest={setDeleteBoard}
                    />
                )
            })}

            {
                showCreateCategoryModal && (
                    <CreateCategory
                        onClose={() => setShowCreateCategoryModal(false)}
                        title={(
                            <FormattedMessage
                                id='SidebarCategories.CategoryMenu.CreateNew'
                                defaultMessage='Create New Category'
                            />
                        )}
                        onCreate={async (name) => {
                            if (!me) {
                                Utils.logError('me not initialized')
                                return
                            }

                            const category: Category = {
                                name,
                                userID: me.id,
                                teamID,
                            } as Category

                            await mutator.createCategory(category)
                            setShowCreateCategoryModal(false)
                        }}
                    />
                )
            }

            {
                showUpdateCategoryModal && (
                    <CreateCategory
                        initialValue={props.categoryBoards.name}
                        title={(
                            <FormattedMessage
                                id='SidebarCategories.CategoryMenu.Update'
                                defaultMessage='Rename Category'
                            />
                        )}
                        onClose={() => setShowUpdateCategoryModal(false)}
                        onCreate={async (name) => {
                            if (!me) {
                                Utils.logError('me not initialized')
                                return
                            }

                            const category: Category = {
                                name,
                                id: props.categoryBoards.id,
                                userID: me.id,
                                teamID,
                            } as Category

                            await mutator.updateCategory(category)
                            setShowUpdateCategoryModal(false)
                        }}
                    />
                )
            }

            { deleteBoard &&
                <DeleteBoardDialog
                    boardTitle={deleteBoard.title}
                    onClose={() => setDeleteBoard(null)}
                    onDelete={onDeleteBoard}
                />
            }

            {
                showDeleteCategoryDialog && <ConfirmationDialogBox dialogBox={deleteCategoryProps}/>
            }
        </div>
    )
}

export default React.memo(SidebarCategory)
