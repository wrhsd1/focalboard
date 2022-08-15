// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React from 'react'

import {createMemoryHistory} from 'history'
import {Router} from 'react-router-dom'

import {render} from '@testing-library/react'

import {Provider as ReduxProvider} from 'react-redux'

import configureStore from 'redux-mock-store'

import {TestBlockFactory} from '../../test/testBlockFactory'

import {wrapIntl} from '../../testUtils'

import SidebarBoardItem from './sidebarBoardItem'

describe('components/sidebarBoardItem', () => {
    const board = TestBlockFactory.createBoard()

    const view = TestBlockFactory.createBoardView(board)
    view.fields.sortOptions = []
    const history = createMemoryHistory()

    const categoryBoards1 = TestBlockFactory.createCategoryBoards()
    categoryBoards1.name = 'Category 1'
    categoryBoards1.boardIDs = [board.id]

    const categoryBoards2 = TestBlockFactory.createCategoryBoards()
    categoryBoards2.name = 'Category 2'

    const categoryBoards3 = TestBlockFactory.createCategoryBoards()
    categoryBoards3.name = 'Category 3'

    const allCategoryBoards = [
        categoryBoards1,
        categoryBoards2,
        categoryBoards3,
    ]

    const state = {
        users: {
            me: {
                id: 'user_id_1',
            },
        },
        boards: {
            current: board.id,
            boards: {
                [board.id]: board,
            },
        },
        views: {
            current: view.id,
            views: {
                [view.id]: view,
            },
        },
        teams: {
            current: {
                id: 'team-id',
            },
        },
    }

    test('sidebar board item', () => {
        const mockStore = configureStore([])
        const store = mockStore(state)

        const component = wrapIntl(
            <ReduxProvider store={store}>
                <Router history={history}>
                    <SidebarBoardItem
                        categoryBoards={categoryBoards1}
                        board={board}
                        allCategories={allCategoryBoards}
                        isActive={true}
                        showBoard={jest.fn()}
                        showView={jest.fn()}
                        onDeleteRequest={jest.fn()}
                    />
                </Router>
            </ReduxProvider>,
        )
        const {container} = render(component)
        expect(container).toMatchSnapshot()
    })

    test('renders default icon if no custom icon set', () => {
        const mockStore = configureStore([])
        const store = mockStore(state)
        const noIconBoard = { ...board, icon: '' }

        const component = wrapIntl(
            <ReduxProvider store={store}>
                <Router history={history}>
                    <SidebarBoardItem
                        categoryBoards={categoryBoards1}
                        board={noIconBoard}
                        allCategories={allCategoryBoards}
                        isActive={true}
                        showBoard={jest.fn()}
                        showView={jest.fn()}
                        onDeleteRequest={jest.fn()}
                    />
                </Router>
            </ReduxProvider>,
        )
        const {container} = render(component)
        expect(container).toMatchSnapshot()
    })
})
