// @flow

import * as React from 'react'
import {Avatar, Box, Button, Divider, Text} from '../../common-adapters'
import {globalStyles, globalColors, globalMargins} from '../../styles'
import {isMobile} from '../../constants/platform'
import PopupMenu, {ModalLessPopupMenu} from '../../common-adapters/popup-menu'

import type {Props} from './index'

const TeamInfo = (props: Props) => (
  <Box style={{...globalStyles.flexBoxColumn, alignItems: 'center'}}>
    <Avatar
      teamname={props.teamname}
      size={40}
      style={{marginTop: globalMargins.small, marginBottom: globalMargins.small}}
    />

    <Box style={{...globalStyles.flexBoxRow, marginTop: globalMargins.xtiny}}>
      <Text type="BodySemibold">{props.teamname}</Text>
    </Box>

    <Text style={isMobile ? styleText : {...styleText, textTransform: 'uppercase'}} type="Body">
      {props.openTeam && 'OPEN '}TEAM
    </Text>

    <Text style={styleText} type="Body">
      {props.memberCount} members
    </Text>

    <Box style={styleDescription}>
      <Text style={styleText} type="Body">{props.description}</Text>
    </Box>

    {!!props.teamJoinError && <Box style={styleDescription}>
      <Text style={{padding: globalMargins.small}} type="BodySmall">Error: {props.teamJoinError}</Text>
    </Box>}

    {!props.youAreInTeam &&
      <Box style={styleDivider}>
        <Button
          onClick={props.onJoinTeam}
          disabled={props.teamJoinSuccess || props.youHaveRequestedAccess}
          label={(props.teamJoinSuccess || props.youHaveRequestedAccess) ? 'Request sent' : props.openTeam ? 'Join team' : 'Request to join'}
          style={{marginTop: globalStyles.small}}
          type={(props.teamJoinSuccess || props.youHaveRequestedAccess) ? 'Secondary' : props.openTeam ? 'Following' : 'Primary'}
        />
      </Box>}

    <Box style={styleDivider}>
      <Divider />
    </Box>
  </Box>
)

const styleDescription = {
  ...globalStyles.flexBoxRow,
  marginBottom: globalMargins.tiny,
  marginLeft: globalMargins.small,
  marginRight: globalMargins.small,
  marginTop: globalMargins.tiny,
}

const styleDivider = {
  ...globalStyles.flexBoxRow,
  marginTop: globalMargins.tiny,
}

const styleText = {
  color: globalColors.black_20,
  fontSize: 11,
}

const TeamInfoWrapper = (props: Props) => {
  const header = {
    title: 'header',
    view: <TeamInfo {...(props: Props)} />,
  }
  let items = []

  return isMobile
    ? <PopupMenu onHidden={props.onHidden} style={{overflow: 'visible'}} header={header} items={items} />
    : <ModalLessPopupMenu
        onHidden={() => {}}
        style={{overflow: 'visible', width: 220}}
        header={header}
        items={items}
      />
}

export default TeamInfoWrapper
